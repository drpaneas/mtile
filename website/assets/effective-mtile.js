/* Storage arithmetic illustrates representation, never predicted GBA timing. */
(() => {
  const slider = document.getElementById('storage-count');
  if (slider) {
    const update = () => {
      const count = Math.max(1, Math.min(32, Number(slider.value) || 16));
      const general = count * 12;
      const numeric = count * 4;
      document.getElementById('storage-count-value').textContent = String(count);
      document.getElementById('storage-general').textContent = `${general} bytes`;
      document.getElementById('storage-numeric').textContent = `${numeric} bytes`;
      document.getElementById('storage-general-bar').style.width = `${general / 384 * 100}%`;
      document.getElementById('storage-numeric-bar').style.width = `${numeric / 384 * 100}%`;
      document.getElementById('storage-feedback').textContent =
        `${count} ${count === 1 ? 'element saves' : 'elements save'} ${general - numeric} bytes of element storage when the numeric representation is eligible.`;
    };
    slider.addEventListener('input', update);
    update();
  }

  // Old deep links still reveal the reference after it moves into <details>.
  const revealHash = () => {
    let id;
    try { id = decodeURIComponent(window.location.hash.slice(1)); }
    catch { return; }
    if (!id) return;
    const target = document.getElementById(id);
    if (!target) return;
    let ancestor = target.parentElement;
    let opened = false;
    while (ancestor) {
      if (ancestor.tagName === 'DETAILS' && !ancestor.open) {
        ancestor.open = true;
        opened = true;
      }
      ancestor = ancestor.parentElement;
    }
    if (opened) target.scrollIntoView({ block: 'start' });
  };
  window.addEventListener('hashchange', revealHash);
  revealHash();
})();
