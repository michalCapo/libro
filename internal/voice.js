// Voice typing extends the existing agent tab: one microphone, inline progress,
// and editable text in the original terminal. It never submits the prompt.
(function () {
  if (window.libroVoice) return;
  let setup = {state: 'installing', message: 'Preparing voice typing…'};
  let current = null;
  let message = '';
  let pollTimer;
  const headers = () => ({'X-Libro-Session': window.__libroWorkspaceSID});
  const target = id => document.getElementById('frame-' + id);
  const available = id => {
    const frame = target(id);
    return frame?.isConnected && !frame.closest('[aria-hidden="true"]') && frame.getClientRects().length > 0;
  };

  function render() {
    const shortcut = window.libroWorkspace?.shortcutFor('voice') || '';
    document.querySelectorAll('[data-voice-button]').forEach(button => {
      const active = current?.indicatorID === button.dataset.voiceButton;
      const state = active ? current.state : setup.state;
      const label = active ? (state === 'recording' ? 'Listening… Release to transcribe' : state === 'permission' ? 'Waiting for microphone…' : 'Transcribing…') :
        message || (state === 'ready' ? '' : state === 'error' ? 'Voice setup failed · Retry' : setup.message);
      button.dataset.state = state;
      button.setAttribute('aria-pressed', String(active && state === 'recording'));
      button.title = state === 'error' ? setup.message : 'Hold to speak' + (shortcut ? ' (' + shortcut + ')' : '') + '. Escape cancels.';
      button.setAttribute('aria-label', button.title);
      button.querySelector('i').textContent = state === 'error' ? 'refresh' : 'mic';
      const status = button.nextElementSibling;
      if (status.textContent !== label) status.textContent = label;
      status.hidden = !label;
    });
  }

  async function poll() {
    clearTimeout(pollTimer);
    try {
      const response = await fetch('/voice/status', {headers: headers()});
      if (!response.ok) throw new Error('Cannot reach voice typing. Click the microphone to retry.');
      setup = await response.json();
    } catch (error) { setup = {state:'error', message:error.message}; }
    render();
    if (setup.state === 'installing' || setup.state === 'idle') pollTimer = setTimeout(poll, 1500);
  }

  async function retry() {
    setup = {state:'installing', message:'Preparing voice typing…'};
    message = ''; render();
    try {
      const response = await fetch('/voice/setup', {method:'POST', headers: headers()});
      if (!response.ok) throw new Error('Could not start voice setup. Click to retry.');
      await poll();
    } catch (error) { setup = {state:'error', message:error.message}; render(); }
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

  async function begin(id, source) {
    const indicatorID = id;
    const selected = window.__libroSelectedApp;
    if (available(selected) && target(selected).querySelector('[data-terminal-app]') &&
        target(selected).closest('[data-workspace-project]') === target(id)?.closest('[data-workspace-project]')) id = selected;
    if (current || !available(id)) return;
    if (setup.state === 'error' || setup.state === 'idle') { await retry(); return; }
    if (setup.state !== 'ready') return;
    message = '';
    const attempt = {id, indicatorID, source, state:'permission', abort:new AbortController(), chunks:[]};
    current = attempt; render();
    try {
      if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) throw new Error('Microphone recording requires the desktop app or localhost.');
      const stream = await navigator.mediaDevices.getUserMedia({audio:{channelCount:1, echoCancellation:true, noiseSuppression:true}, video:false});
      attempt.stream = stream;
      // The key may have been released while the OS permission prompt was open.
      if (current !== attempt || !available(id)) { releaseResources(attempt); if (current === attempt) cancel(); return; }
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
      if (samples.length < 1600) throw new Error('Hold the microphone a little longer, then speak.');
      const energy = samples.reduce((sum, value) => sum + value*value, 0) / samples.length;
      if (energy < 0.00001) throw new Error('No speech heard. Check your microphone and try again.');
      const response = await fetch('/voice/transcribe', {method:'POST', headers:{...headers(), 'Content-Type':'audio/wav'}, body:wav(samples), signal:attempt.abort.signal});
      if (!response.ok) throw new Error(await response.text());
      const result = await response.json();
      if (current !== attempt) return;
      if (!available(attempt.id)) { cancel(); return; }
      if (!result.text?.trim()) throw new Error('No speech recognized. Please try again.');
      if (!window.__libroSendPageToolPrompt?.(result.text, false, attempt.id)) throw new Error('Terminal disconnected. Please reconnect and try again.');
      current = null; message = 'Text inserted · Review and press Enter';
      window.__libroFocusAppByID?.(attempt.id);
      render();
    } catch (error) { fail(attempt, error); }
  }

  function mount(group, id) {
    const button = document.createElement('button');
    button.type = 'button'; button.className = 'ws-button ws-voice-button'; button.dataset.voiceButton = id;
    const icon = document.createElement('i'); icon.className = 'material-icons-round'; icon.setAttribute('aria-hidden', 'true'); button.append(icon);
    const status = document.createElement('span'); status.className = 'ws-voice-status'; status.setAttribute('role', 'status');
    button.onpointerdown = event => {
      if (event.button !== 0) return;
      event.preventDefault(); void begin(id, {pointer:event.pointerId});
    };
    button.onkeydown = event => {
      if (![' ', 'Enter'].includes(event.key)) return;
      event.preventDefault(); event.stopPropagation();
      if (!event.repeat) void begin(id, {code:event.code});
    };
    // Assistive technology can activate the same control without holding a pointer.
    button.onclick = event => { if (event.detail === 0) { if (current) end(); else void begin(id, {toggle:true}); } };
    group.prepend(button); group.append(status); render();
  }

  window.addEventListener('keyup', event => {
    if (current?.source.code && (event.code === current.source.code || ['Control','Alt','Shift','Meta'].includes(event.key))) {
      event.preventDefault(); event.stopImmediatePropagation(); end();
    }
  }, true);
  window.addEventListener('pointerup', event => { if (current?.source.pointer === event.pointerId) end(); }, true);
  window.addEventListener('pointercancel', event => { if (current?.source.pointer === event.pointerId) cancel(); }, true);
  window.addEventListener('keydown', event => {
    if (event.key === 'Escape' && current) { event.preventDefault(); event.stopImmediatePropagation(); cancel(); }
  }, true);
  window.addEventListener('blur', cancel);
  document.addEventListener('visibilitychange', () => { if (document.hidden) cancel(); });
  new MutationObserver(() => { if (current && !available(current.id)) cancel(); }).observe(document.getElementById('libro-workspace'), {subtree:true, childList:true, attributes:true, attributeFilter:['aria-hidden']});
  window.libroVoice = {begin, cancel, mount, refresh:render};
  void poll();
})();
