// Keep unsaved appearance text local. Delegated input handling survives htmx
// navigation without per-page listeners or requests while typing.
function updatePreview(editor) {
  const name = editor.querySelector('[name="name"]').value.trim();
  const introduction = editor.querySelector('[name="introduction"]').value.trim();
  editor.querySelector('[data-preview-name]').textContent = name;
  editor.querySelector('[data-login-site-name]').textContent = name;
  const custom = editor.querySelector('[data-login-introduction]');
  custom.textContent = introduction;
  custom.hidden = !introduction;
  editor.querySelector('[data-login-default]').hidden = Boolean(introduction);
}

function mountPreviews() {
  for (const editor of document.querySelectorAll('[data-appearance-editor]')) {
    editor.querySelector('[data-preview-button]').hidden = true;
    editor.querySelector('[data-preview-status]').textContent =
      'Preview updates as you type. Changes apply when you save.';
    updatePreview(editor);
  }
}

document.addEventListener('input', (event) => {
  const editor = event.target.closest('[data-appearance-editor]');
  if (editor) updatePreview(editor);
});
mountPreviews();
document.addEventListener('htmx:afterSettle', mountPreviews);
document.addEventListener('htmx:historyRestore', mountPreviews);
