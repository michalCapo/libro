// Electron captures and normalizes images; the backend saves the files.
window.__libroSavePageToolImages = async (id, images) => {
  const form = new FormData();
  for (const data of images) {
    if (!/^data:image\/png;base64,/.test(data)) throw new Error('Invalid page image.');
    const response = await fetch(data);
    form.append('images', await response.blob(), 'image.png');
  }
  const query = new URLSearchParams({sid:window.__libroWorkspaceSID, id});
  const response = await fetch('/attachments?' + query, {method:'POST', body:form});
  if (!response.ok) throw new Error(await response.text());
  return (await response.json()).paths;
};
