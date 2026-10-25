/* Optional keyboard selection; native links work without JavaScript. */
(() => {
  const menu = document.querySelector('.game-menu');
  const choices = [...menu.querySelectorAll('a')];
  function select(link) {
    choices.forEach(item => item.classList.toggle('selected', item === link));
  }
  choices.forEach(link => {
    link.addEventListener('focus', () => select(link));
    link.addEventListener('pointerenter', () => select(link));
  });
  menu.addEventListener('keydown', event => {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const index = choices.indexOf(document.activeElement);
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? choices.length - 1
      : (index + (event.key === 'ArrowDown' ? 1 : -1) + choices.length) % choices.length;
    choices[next].focus();
  });
})();
