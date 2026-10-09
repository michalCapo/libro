// Shared by the workspace and browser guests. Captures an editable selection
// without exposing the field's contents to the host.
(function () {
  if (window.libroVoiceInput) return;
  let captured;
  let previous;
  let nextToken = 0;

  function editable(element) {
    if (!element || element.disabled || element.readOnly || element.closest?.('[data-terminal], [inert]')) return false;
    return element.isContentEditable || element.tagName === 'TEXTAREA' ||
      element.tagName === 'INPUT' && ['text', 'search', 'email', 'url', 'tel', 'password', 'number'].includes(element.type);
  }

  function focused() {
    let element = document.activeElement;
    while (element?.shadowRoot?.activeElement) element = element.shadowRoot.activeElement;
    if (element?.closest?.('[data-voice-control]')) element = previous;
    return editable(element) ? element : null;
  }

  document.addEventListener('focusin', event => {
    const element = event.composedPath()[0];
    if (!element.closest?.('[data-voice-control]')) previous = editable(element) ? element : null;
  }, true);

  function capture() {
    const element = focused();
    captured = null;
    if (!element) return 0;
    const selection = element.ownerDocument.getSelection();
    captured = {element, token:++nextToken, start:element.selectionStart, end:element.selectionEnd,
      range:element.isContentEditable && selection?.rangeCount ? selection.getRangeAt(0).cloneRange() : null};
    return captured.token;
  }

  function available(token) {
    const element = captured?.element;
    return captured?.token === token && editable(element) && element.isConnected && element.getClientRects().length > 0;
  }

  function insert(token, text) {
    if (!available(token)) return false;
    const {element, start, end, range} = captured;
    const doc = element.ownerDocument;
    element.focus();
    if (element.isContentEditable) {
      const selection = doc.getSelection();
      if (range && element.contains(range.commonAncestorContainer)) {
        selection.removeAllRanges(); selection.addRange(range);
      }
      // Native editing keeps rich text editors' models and undo histories in sync.
      if (!doc.execCommand('insertText', false, text)) return false;
    } else {
      if (start !== null && start !== undefined) {
        element.setSelectionRange(start, end);
        if (doc.execCommand?.('insertText', false, text)) return true;
        element.setRangeText(text, start, end, 'end');
      } else {
        const prototype = element.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
        Object.getOwnPropertyDescriptor(prototype, 'value').set.call(element, element.value + text);
      }
      element.dispatchEvent(new InputEvent('input', {bubbles:true, composed:true, inputType:'insertText', data:text}));
    }
    return true;
  }

  window.libroVoiceInput = {capture, available, insert};
})();
