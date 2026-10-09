// Shared voice typing inserts into the input or terminal captured at recording
// start. Dictation never submits a form or terminal command.
(function () {
  if (window.libroVoice) return;
  let setup = {state: 'ready'};
  let current = null;
  let message = '';
  // Status notices hide after a few seconds; the button keeps showing the state.
  let shown = '';
  let expired = '';
  let hideTimer;
  const headers = () => ({'X-Libro-Session': window.__libroWorkspaceSID});
  const target = id => document.getElementById('frame-' + id);
  const available = id => {
    const frame = target(id);
    return frame?.isConnected && !frame.closest('[aria-hidden="true"]') && frame.getClientRects().length > 0;
  };

  async function destination(id) {
    const token = window.libroVoiceInput?.capture();
    if (token) return {
      available:() => window.libroVoiceInput.available(token),
      insert:text => window.libroVoiceInput.insert(token, text),
    };
    const selected = window.__libroSelectedApp;
    const frame = target(selected);
    if (available(selected)) {
      if (frame.querySelector('[data-webview-app], [data-browser-iframe-app]')) {
        const input = await window.__libroCaptureBrowserVoiceInput?.(selected);
        if (input) return {id:selected, ...input};
      }
      if (frame.querySelector('[data-terminal-app]')) id = selected;
      else id ||= frame.closest('[data-workspace-project]')?.querySelector('[data-dock="center"]')?.dataset.appId;
    }
    if (!available(id)) return null;
    return {id, available:() => available(id), insert:text => {
      const inserted = window.__libroSendPageToolPrompt?.(' ' + text, false, id);
      if (inserted) window.__libroFocusAppByID?.(id);
      return inserted;
    }};
  }

  const valid = attempt => attempt.project === window.__libroActiveProject && attempt.target?.available();

  function render() {
    const shortcut = window.libroWorkspace?.shortcutFor('voice') || '';
    const active = !!current;
    const state = active ? current.state : 'ready';
    let label = active ? (state === 'recording' ? 'Listening… Press again to transcribe' : state === 'permission' ? 'Waiting for microphone…' : 'Transcribing…') : message;
    if (label !== shown) {
      shown = label; expired = '';
      clearTimeout(hideTimer);
      if (label) hideTimer = setTimeout(() => { expired = label; render(); }, 4000);
    }
    if (label === expired) label = '';
    document.querySelectorAll('[data-voice-button]').forEach(button => {
      button.dataset.state = state;
      button.setAttribute('aria-pressed', String(active && state === 'recording'));
      button.title = 'Press to start or stop voice typing' + (shortcut ? ' (' + shortcut + ')' : '') + '. Escape cancels.';
      button.setAttribute('aria-label', button.title);
      button.querySelector('i').textContent = 'mic';
      const status = button.parentElement.querySelector('.ws-voice-status');
      if (status.textContent !== label) status.textContent = label;
      status.hidden = !label;
      if (label) {
        if (button.getBoundingClientRect) {
          const bounds = button.getBoundingClientRect();
          status.style.top = Math.max(32, Math.min(window.innerHeight - 32, bounds.top + bounds.height / 2)) + 'px';
        }
        if (!status.matches?.(':popover-open')) status.showPopover?.();
      } else status.hidePopover?.();
    });
  }

  // Status only reports whether an OpenRouter key is set; it is shown when
  // the user tries to dictate.
  async function poll() {
    try {
      const response = await fetch('/voice/status', {headers: headers()});
      if (!response.ok) throw new Error('Cannot reach voice typing. Please try again.');
      setup = await response.json();
    } catch (error) { setup = {state:'error', message:error.message}; }
    if (setup.state === 'ready') message = '';
    render();
  }

  function releaseResources(attempt) {
    clearTimeout(attempt.timer);
    attempt.stream?.getTracks().forEach(track => track.stop());
  }

  function cancel() {
    const attempt = current;
    if (!attempt) return;
    current = null;
    attempt.abort.abort();
    if (attempt.recorder?.state === 'recording') attempt.recorder.stop();
    releaseResources(attempt);
    message = 'Recording canceled'; render();
  }

  function fail(attempt, error) {
    if (current !== attempt) return;
    current = null;
    releaseResources(attempt);
    message = error.name === 'NotAllowedError' ? 'Microphone access denied. Allow it in system settings and try again.' :
      error.name === 'NotFoundError' ? 'No microphone found. Connect one and try again.' : error.message;
    render();
  }

  async function toggle(id) {
    if (current) { end(); return; }
    shown = '';
    if (setup.state !== 'ready') await poll();
    if (setup.state !== 'ready') { message = setup.message; render(); return; }
    message = '';
    const attempt = {project:window.__libroActiveProject, state:'permission', abort:new AbortController(), chunks:[]};
    current = attempt; render();
    try {
      attempt.target = await destination(id);
      if (current !== attempt) return;
      if (!attempt.target) throw new Error('Focus an input field or terminal before starting voice typing.');
      if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) throw new Error('Microphone recording requires the desktop app or localhost.');
      const stream = await navigator.mediaDevices.getUserMedia({audio:{channelCount:1, echoCancellation:true, noiseSuppression:true}, video:false});
      attempt.stream = stream;
      // Recording may have been canceled while the OS permission prompt was open.
      if (current !== attempt || !valid(attempt)) { releaseResources(attempt); if (current === attempt) cancel(); return; }
      const recorder = new MediaRecorder(stream);
      attempt.recorder = recorder;
      recorder.ondataavailable = event => { if (event.data.size) attempt.chunks.push(event.data); };
      recorder.onerror = () => fail(attempt, new Error('Recording failed. Check your microphone and try again.'));
      recorder.onstop = () => { releaseResources(attempt); if (current === attempt) void transcribe(attempt); };
      stream.getAudioTracks()[0].onended = () => { if (current === attempt && attempt.state === 'recording') cancel(); };
      recorder.start(250);
      attempt.state = 'recording';
      attempt.timer = setTimeout(end, 60000);
      render();
    } catch (error) { fail(attempt, error); }
  }

  function end() {
    const attempt = current;
    if (!attempt) return;
    if (attempt.state === 'permission') { cancel(); return; }
    if (attempt.state !== 'recording') return;
    attempt.state = 'transcribing';
    clearTimeout(attempt.timer);
    attempt.recorder.stop();
    releaseResources(attempt);
    render();
  }

  function wav(samples) {
    const buffer = new ArrayBuffer(44 + samples.length * 2);
    const view = new DataView(buffer);
    const text = (offset, value) => { for (let i = 0; i < value.length; i++) view.setUint8(offset+i, value.charCodeAt(i)); };
    text(0, 'RIFF'); view.setUint32(4, buffer.byteLength-8, true); text(8, 'WAVEfmt ');
    view.setUint32(16, 16, true); view.setUint16(20, 1, true); view.setUint16(22, 1, true);
    view.setUint32(24, 16000, true); view.setUint32(28, 32000, true); view.setUint16(32, 2, true); view.setUint16(34, 16, true);
    text(36, 'data'); view.setUint32(40, samples.length*2, true);
    samples.forEach((value, i) => view.setInt16(44+i*2, Math.max(-1, Math.min(1, value)) * (value < 0 ? 32768 : 32767), true));
    return buffer;
  }

  async function transcribe(attempt) {
    try {
      const blob = new Blob(attempt.chunks, {type:attempt.recorder.mimeType});
      attempt.chunks = [];
      if (!blob.size || blob.size > 8*1024*1024) throw new Error('Could not read the recording. Please try again.');
      const audio = new AudioContext({sampleRate:16000});
      let decoded;
      try { decoded = await audio.decodeAudioData(await blob.arrayBuffer()); }
      finally { await audio.close(); }
      if (current !== attempt) return;
      const samples = decoded.getChannelData(0).subarray(0, 16000*60);
      if (samples.length < 1600) throw new Error('Speak a little longer before stopping the recording.');
      const energy = samples.reduce((sum, value) => sum + value*value, 0) / samples.length;
      if (energy < 0.00001) throw new Error('No speech heard. Check your microphone and try again.');
      const response = await fetch('/voice/transcribe', {method:'POST', headers:{...headers(), 'Content-Type':'audio/wav'}, body:wav(samples), signal:attempt.abort.signal});
      if (!response.ok) throw new Error(await response.text());
      const result = await response.json();
      if (current !== attempt) return;
      if (!valid(attempt)) { cancel(); return; }
      if (!result.text?.trim()) throw new Error('No speech recognized. Please try again.');
      attempt.state = 'inserting';
      if (!await attempt.target.insert(result.text)) throw new Error('The input is no longer available. Focus it and try again.');
      if (current !== attempt) return;
      current = null; message = 'Text inserted · Review before sending';
      render();
    } catch (error) { fail(attempt, error); }
  }

  // Clicking the microphone must preserve the focused field and its selection.
  document.addEventListener('pointerdown', event => {
    if (event.target.closest?.('[data-voice-button]')) event.preventDefault();
  }, true);

  window.addEventListener('keydown', event => {
    if (event.key === 'Escape' && current) { event.preventDefault(); event.stopImmediatePropagation(); cancel(); }
    if (event.repeat && [' ', 'Enter'].includes(event.key) && event.target.closest?.('[data-voice-button]')) event.preventDefault();
  }, true);
  // Refocusing a browser input after insertion can blur the host renderer.
  window.addEventListener('blur', () => { if (current?.state !== 'inserting') cancel(); });
  document.addEventListener('visibilitychange', () => { if (document.hidden) cancel(); });
  new MutationObserver(() => { if (current?.target && !valid(current)) cancel(); }).observe(document.getElementById('libro-workspace'), {subtree:true, childList:true, attributes:true, attributeFilter:['aria-hidden']});
  window.libroVoice = {toggle, cancel, poll, refresh:render};
  void poll();
})();
