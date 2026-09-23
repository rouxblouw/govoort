document.querySelectorAll('.c-card').forEach(el => {
  if (el.dataset.cardInit) return;
  el.dataset.cardInit = 'true';

  let count = parseInt(el.dataset.likeCount, 10) || 0;
  const threshold = parseInt(el.dataset.threshold, 10) || Infinity;
  const btn = el.querySelector('.c-card__like-btn');
  const countEl = el.querySelector('.c-card__like-count');

  btn.addEventListener('click', () => {
    count++;
    countEl.textContent = count;
    if (count >= threshold) {
      el.classList.add('c-card--popular');
    }
  });
});