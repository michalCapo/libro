import { Editor } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import { Markdown } from '@tiptap/markdown';
import Image from '@tiptap/extension-image';
import Placeholder from '@tiptap/extension-placeholder';
import { TableKit } from '@tiptap/extension-table';
import TaskList from '@tiptap/extension-task-list';
import TaskItem from '@tiptap/extension-task-item';

const imageID = () => Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('');

// Markdown stores stable attachment references; image bytes stay in the existing images field.
export function create({ element, note, onChange, onBusy, onError }) {
  const attachments = new Map();
  let content = note.body;
  for (const image of note.images || []) {
    const id = image.id || imageID();
    const src = 'note-image:' + id;
    attachments.set(src, image.data);
    if (!image.id) content += `\n\n![Screenshot](${src})`;
  }
  const toolbar = document.createElement('div');
  toolbar.className = 'ws-note-formatting';
  toolbar.setAttribute('role', 'group');
  toolbar.setAttribute('aria-label', 'Text formatting');
  const body = document.createElement('div');
  element.append(toolbar, body);
  let ready = false;
  const editor = new Editor({
    element: body,
    extensions: [
      StarterKit.configure({ link: { openOnClick: false, autolink: false } }),
      Markdown, TableKit, TaskList, TaskItem.configure({ nested: true }),
      Placeholder.configure({ placeholder: 'Describe the task…' }),
      Image.extend({
        renderHTML({ node }) {
          const data = attachments.get(node.attrs.src);
          return data
            ? ['img', { src: data, alt: node.attrs.alt || 'Screenshot', title: node.attrs.title }]
            : ['span', { class: 'ws-note-image-missing' }, node.attrs.alt || 'Image unavailable'];
        },
      }),
    ],
    content,
    contentType: 'markdown',
    editorProps: {
      attributes: { class: 'ws-note-body', role: 'textbox', 'aria-label': 'Note', 'aria-multiline': 'true' },
      handlePaste(view, event) {
        const files = Array.from(event.clipboardData?.files || []).filter(file => file.type.startsWith('image/'));
        if (files.length) {
          event.preventDefault();
          void insertImages(files);
          return true;
        }
        // Parse clipboard text as Markdown. Never load images or HTML from external clipboard sources.
        const text = event.clipboardData?.getData('text/plain');
        if (text) {
          event.preventDefault();
          if (editor.isActive('codeBlock')) editor.commands.insertContent({ type: 'text', text });
          else editor.commands.insertContent(text, { contentType: 'markdown' });
          return true;
        }
        return true;
      },
      handleDrop(view, event, slice, moved) {
        if (moved) return false;
        const files = Array.from(event.dataTransfer?.files || []);
        if (files.length) {
          event.preventDefault();
          const pos = view.posAtCoords({ left: event.clientX, top: event.clientY });
          if (pos) editor.commands.setTextSelection(pos.pos);
          void insertImages(files);
        }
        return true;
      },
    },
    onUpdate: () => onChange(serialize()),
    onTransaction: () => { if (ready) updateToolbar(); },
  });

  function serialize() {
    const images = new Map();
    editor.state.doc.descendants(node => {
      if (node.type.name === 'image' && attachments.has(node.attrs.src)) {
        images.set(node.attrs.src, { id: node.attrs.src.slice('note-image:'.length), data: attachments.get(node.attrs.src) });
      }
    });
    return { body: editor.getMarkdown(), images: [...images.values()] };
  }

  let reading = false;
  async function insertImages(files) {
    if (reading || !editor.isEditable) return;
    reading = true;
    onBusy(true);
    // Map the paste position through edits made while FileReader is running.
    let bookmark = editor.state.selection.getBookmark();
    const mapPosition = ({ transaction }) => { bookmark = bookmark.map(transaction.mapping); };
    editor.on('transaction', mapPosition);
    try {
      const added = [];
      for (const file of files) {
        if (!['image/png', 'image/jpeg', 'image/gif'].includes(file.type)) throw new Error('Use a PNG, JPEG, or GIF image.');
        if (file.size > 4 * 1024 * 1024) throw new Error('Each image must be 4 MB or smaller.');
        const data = await new Promise((resolve, reject) => {
          const reader = new FileReader();
          reader.onload = () => resolve(reader.result);
          reader.onerror = () => reject(new Error('Could not read image.'));
          reader.readAsDataURL(file);
        });
        added.push({ src: 'note-image:' + imageID(), data });
      }
      if (editor.isDestroyed) return;
      const total = [...serialize().images, ...added].reduce((size, image) => size + atob(image.data.split(',')[1]).length, 0);
      if (total > 4 * 1024 * 1024) throw new Error('Images must total 4 MB or less.');
      const selection = bookmark.resolve(editor.state.doc);
      for (const image of added) attachments.set(image.src, image.data);
      editor.chain().focus().insertContentAt({ from: selection.from, to: selection.to }, added.map(image => ({ type: 'image', attrs: { src: image.src, alt: 'Screenshot' } }))).run();
    } catch (error) {
      if (!editor.isDestroyed) onError(error.message);
    } finally {
      editor.off('transaction', mapPosition);
      reading = false;
      if (!editor.isDestroyed) onBusy(false);
    }
  }

  const format = document.createElement('select');
  format.setAttribute('aria-label', 'Text style');
  for (const [value, label] of [['0', 'Text'], ['1', 'Heading 1'], ['2', 'Heading 2'], ['3', 'Heading 3']]) {
    const option = document.createElement('option'); option.value = value; option.textContent = label; format.append(option);
  }
  format.onchange = () => {
    const chain = editor.chain().focus();
    (format.value === '0' ? chain.setParagraph() : chain.toggleHeading({ level: Number(format.value) })).run();
  };
  toolbar.append(format);
  const controls = [];
  function control(label, icon, command, active) {
    const button = document.createElement('button');
    button.type = 'button'; button.title = label; button.setAttribute('aria-label', label);
    const glyph = document.createElement('span'); glyph.className = 'material-icons-round'; glyph.textContent = icon; glyph.setAttribute('aria-hidden', 'true');
    button.append(glyph);
    button.onmousedown = event => event.preventDefault();
    button.onclick = command;
    toolbar.append(button);
    controls.push({ button, active });
  }
  for (const [label, icon, command, active] of [
    ['Bold', 'format_bold', 'toggleBold', 'bold'],
    ['Italic', 'format_italic', 'toggleItalic', 'italic'],
    ['Strikethrough', 'strikethrough_s', 'toggleStrike', 'strike'],
    ['Bullet list', 'format_list_bulleted', 'toggleBulletList', 'bulletList'],
    ['Numbered list', 'format_list_numbered', 'toggleOrderedList', 'orderedList'],
    ['Checklist', 'checklist', 'toggleTaskList', 'taskList'],
    ['Quote', 'format_quote', 'toggleBlockquote', 'blockquote'],
    ['Code block', 'code', 'toggleCodeBlock', 'codeBlock'],
  ]) control(label, icon, () => editor.chain().focus()[command]().run(), active);
  const upload = document.createElement('input');
  upload.type = 'file'; upload.accept = 'image/png,image/jpeg,image/gif'; upload.multiple = true; upload.hidden = true;
  upload.onchange = () => { void insertImages(Array.from(upload.files)); upload.value = ''; };
  control('Insert image', 'attach_file', () => upload.click());
  control('Undo', 'undo', () => editor.chain().focus().undo().run());
  control('Redo', 'redo', () => editor.chain().focus().redo().run());
  element.append(upload);
  function updateToolbar() {
    // Initial editor construction can fire transactions before the toolbar exists.
    if (!element.isConnected) return;
    format.value = String(editor.getAttributes('heading').level || 0);
    for (const { button, active } of controls) {
      if (active) button.setAttribute('aria-pressed', String(editor.isActive(active)));
    }
  }
  ready = true;
  updateToolbar();
  return {
    serialize,
    setEditable(value) {
      editor.setEditable(value, false);
      toolbar.querySelectorAll('button,select').forEach(control => { control.disabled = !value; });
    },
    destroy() { editor.destroy(); },
  };
}
