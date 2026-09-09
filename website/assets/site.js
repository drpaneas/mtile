/* Progressive enhancement: all guide pages and links work without JavaScript. */
(() => {
  const input = document.querySelector('#guide-search');
  const results = document.querySelector('#search-results');
  const list = document.querySelector('#search-list');
  const status = document.querySelector('#search-status');
  const root = new URL(document.body.dataset.root || './', window.location.href);
  const index = window.MTILE_SEARCH || [];
  const normalize = text => text.toLowerCase().replace(/[()]/g, '').trim();
  const documents = index.map(page => ({ ...page, haystack: normalize(`${page.title} ${page.description} ${page.category} ${page.text}`) }));
  function search() {
    const query = normalize(input.value);
    list.replaceChildren();
    results.hidden = !query;
    if (!query) return;
    const terms = query.split(/\s+/);
    const matches = documents.filter(page => terms.every(term => page.haystack.includes(term)))
      .sort((a, b) => rank(b, query) - rank(a, query) || a.title.localeCompare(b.title));
    status.textContent = matches.length ? `${matches.length} result${matches.length === 1 ? '' : 's'}${matches.length > 20 ? ' — showing the first 20' : ''}.` : 'No matches. Try “sprite”, “buttons”, “sound”, or a function name.';
    for (const match of matches.slice(0, 20)) {
      const item = document.createElement('li');
      const link = document.createElement('a');
      link.href = new URL(match.url, root).href;
      const title = document.createElement('strong');
      title.textContent = match.title;
      const description = document.createElement('small');
      description.textContent = match.description;
      link.append(title, description);
      item.append(link);
      list.append(item);
    }
  }
  function rank(page, query) {
    const title = normalize(page.title);
    return (title === query ? 100 : 0) + (title.startsWith(query) ? 30 : 0) + (title.includes(query) ? 20 : 0) + (normalize(page.description).includes(query) ? 5 : 0);
  }
  function closeSearch() { input.value = ''; results.hidden = true; input.focus(); }
  input.addEventListener('input', search);
  document.querySelector('#close-search').addEventListener('click', closeSearch);
  const menu = document.querySelector('.menu-toggle');
  function setMenu(open) {
    document.body.classList.toggle('menu-open', open);
    menu.setAttribute('aria-expanded', String(open));
  }
  menu.addEventListener('click', () => setMenu(menu.getAttribute('aria-expanded') !== 'true'));
  document.addEventListener('keydown', event => {
    const editing = event.target.matches('input, textarea, [contenteditable="true"]');
    if (event.key === '/' && !editing && !event.metaKey && !event.ctrlKey && !event.altKey) {
      event.preventDefault(); input.focus();
    }
    if (event.key === 'Escape') {
      if (!results.hidden) closeSearch();
      if (menu.getAttribute('aria-expanded') === 'true') { setMenu(false); menu.focus(); }
    }
    if (event.key === 'ArrowDown' && event.target === input && !results.hidden) {
      const first = list.querySelector('a');
      if (first) { event.preventDefault(); first.focus(); }
    }
  });
  document.addEventListener('click', event => {
    if (!event.target.closest('#sidebar, .menu-toggle')) setMenu(false);
  });
  for (const pre of document.querySelectorAll('.prose pre')) {
    const wrapper = document.createElement('div');
    wrapper.className = 'code-wrap';
    pre.before(wrapper); wrapper.append(pre);
    const button = document.createElement('button');
    button.type = 'button'; button.className = 'copy-code'; button.textContent = 'Copy';
    button.setAttribute('aria-label', 'Copy code');
    button.addEventListener('click', async () => {
      try {
        if (!navigator.clipboard) throw new Error('Clipboard unavailable');
        await navigator.clipboard.writeText(pre.textContent);
        button.textContent = 'Copied!';
      } catch {
        const selection = window.getSelection();
        const range = document.createRange(); range.selectNodeContents(pre);
        selection.removeAllRanges(); selection.addRange(range);
        button.textContent = 'Selected — copy with Ctrl/Cmd+C';
      }
      setTimeout(() => { button.textContent = 'Copy'; }, 2500);
    });
    wrapper.append(button);
  }
})();
