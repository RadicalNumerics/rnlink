(() => {
  const form = document.getElementById('create-form');
  if (form) {
    const short = form.elements.short;
    const destination = form.elements.long;
    const description = form.elements.description;
    function update() {
      document.getElementById('create-fields').hidden = ![short, destination, description].some(input => input.value.length);
      description.setCustomValidity(description.value.trim() ? '' : 'Please add a description.');
      document.getElementById('save-link').disabled = ![short, destination, description].every(input => input.value.trim() && input.validity.valid);
    }
    form.addEventListener('input', update);
    window.addEventListener('pageshow', update);
    update();
  }
  const other = document.getElementById('other-links');
  const popular = document.getElementById('popular-links');
  const toggle = document.getElementById('toggle-popular');
  const popularRows = Array.from(popular.querySelectorAll('tr[data-path]'));
  let pinned = true;
  let sort = 'path', ascending = true;
  const labels = {path: 'A–Z', date: 'Date added', user: 'Added by', clicks: 'Clicks'};
  function sortRows() {
    const rows = Array.from(other.querySelectorAll('tr[data-path]'));
    rows.sort((a, b) => {
      const comparison = ['date', 'clicks'].includes(sort)
        ? Number(a.dataset[sort]) - Number(b.dataset[sort])
        : a.dataset[sort].localeCompare(b.dataset[sort], undefined, {sensitivity: 'base', numeric: true});
      return (ascending ? comparison : -comparison) || a.dataset.path.localeCompare(b.dataset.path);
    });
    other.append(...rows);
    const empty = other.querySelector('.empty-state');
    if (empty) empty.closest('tr').hidden = rows.length > 0;
    document.getElementById('sort-status').textContent = `${pinned ? 'Other' : 'All'} links · ${labels[sort]} ${ascending ? '↑' : '↓'}`;
  }
  function setPinned(value) {
    pinned = value;
    (pinned ? popular : other).append(...popularRows);
    toggle.setAttribute('aria-pressed', String(pinned));
    toggle.querySelector('.pin-state').textContent = pinned ? 'Pinned' : 'Off';
    document.getElementById('directory-table-scroll').classList.toggle('popular-off', !pinned);
    popular.hidden = !pinned;
    document.querySelector('.table-divider').hidden = !pinned;
    sortRows();
  }
  toggle.addEventListener('click', () => {
    setPinned(!pinned);
    try { localStorage.setItem('rnlink-pin-popular', String(pinned)); } catch (_) {}
  });
  try { if (localStorage.getItem('rnlink-pin-popular') === 'false') setPinned(false); } catch (_) {}
  document.querySelectorAll('[data-sort]').forEach(button => {
    button.addEventListener('click', () => {
      const key = button.dataset.sort;
      ascending = sort === key ? !ascending : !['date', 'clicks'].includes(key);
      sort = key;
      sortRows();
      document.querySelectorAll('[data-sort]').forEach(control => {
        const active = control === button;
        control.closest('th').setAttribute('aria-sort', active ? (ascending ? 'ascending' : 'descending') : 'none');
        control.querySelector('.sort-arrow').textContent = active ? (ascending ? '↑' : '↓') : '↕';
      });
    });
  });
})();
