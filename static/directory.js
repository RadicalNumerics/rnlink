(() => {
  const form = document.getElementById('create-form');
  const other = document.getElementById('other-links');
  const popular = document.getElementById('popular-links');
  const toggle = document.getElementById('toggle-popular');
  let popularRows = Array.from(popular.querySelectorAll('tr[data-path]'));
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

  function refreshRanking() {
    const rows = Array.from(document.querySelectorAll('tr[data-path]'));
    rows.sort((a, b) => Number(b.dataset.clicks) - Number(a.dataset.clicks)
      || (a.dataset.path < b.dataset.path ? -1 : a.dataset.path > b.dataset.path ? 1 : 0));
    popularRows = rows.slice(0, 10);
    rows.forEach((row, index) => {
      row.classList.toggle('popular-row', index < 10);
      const badge = document.createElement('span');
      badge.className = index < 10 ? 'rank-badge' : 'row-mark';
      badge.textContent = index < 10 ? String(index + 1) : '↗';
      if (index < 10) badge.setAttribute('aria-label', `Popularity rank ${index + 1}`);
      row.querySelector('.rank-cell').replaceChildren(badge);
    });
    other.append(...rows);
    setPinned(pinned);
    document.querySelector('.total-count').textContent = `${rows.length} ${rows.length === 1 ? 'link' : 'links'}`;
    const empty = other.querySelector('.empty-state');
    if (empty) empty.textContent = rows.length ? "You're all caught up — every link is listed above." : 'Add your first team link above.';
  }
  if (form) {
    const short = form.elements.short;
    const destination = form.elements.long;
    const description = form.elements.description;
    const save = document.getElementById('save-link');
    const feedback = document.getElementById('create-feedback');
    let saving = false, timer, version = 0, checked = '', serverError = '';
    const key = () => JSON.stringify([short.value, destination.value]);
    function message(text, error = false) {
      feedback.textContent = text;
      feedback.hidden = !text;
      feedback.classList.toggle('is-error', error);
    }
    function update() {
      document.getElementById('create-fields').hidden = ![short, destination, description].some(input => input.value.length);
      let problem = '';
      if (/\s/.test(short.value)) problem = 'Names cannot contain spaces. Use a dash or underscore instead.';
      else if (short.value && !/^\w[\w.\-]*$/.test(short.value)) problem = 'Start with a letter, number, or underscore; use only letters, numbers, underscores, dashes, and periods.';
      short.setCustomValidity(problem);
      description.setCustomValidity(description.value.trim() ? '' : 'Please add a description.');
      save.disabled = saving || checked !== key() || !!serverError || ![short, destination, description].every(input => input.value.trim() && input.validity.valid);
      return problem;
    }
    async function validate() {
      const current = ++version;
      const fingerprint = key();
      const problem = update();
      if (problem || !short.value || !destination.value.trim()) {
        message(problem, !!problem);
        return;
      }
      message('Checking name and destination…');
      const body = new URLSearchParams(new FormData(form));
      body.set('validate', 'true');
      try {
        const response = await fetch('/', {method: 'POST', headers: {'Accept': 'application/json', 'X-RN-Inline': 'true'}, body});
        const error = response.ok ? '' : (await response.text()).trim();
        if (current !== version || fingerprint !== key()) return;
        checked = fingerprint;
        serverError = error;
        message(error || (destination.value.includes('{{') ? 'Template syntax and example expansion are valid.' : ''), !!error);
      } catch (_) {
        if (current !== version) return;
        serverError = 'Could not validate. Edit a field to retry.';
        message(serverError, true);
      }
      update();
    }
    form.addEventListener('input', event => {
      if (saving) return;
      if (event.target === short || event.target === destination) {
        ++version;
        checked = '';
        serverError = '';
        clearTimeout(timer);
        const problem = update();
        message(problem, !!problem);
        timer = setTimeout(validate, 300);
      } else update();
    });
    form.addEventListener('submit', async event => {
      event.preventDefault();
      if (saving || save.disabled || !form.reportValidity()) return;
      const body = new URLSearchParams(new FormData(form));
      saving = true;
      ++version;
      clearTimeout(timer);
      save.textContent = 'Saving…';
      [short, destination, description].forEach(input => input.readOnly = true);
      update();
      message('Saving link…');
      try {
        const response = await fetch('/', {method: 'POST', headers: {'Accept': 'application/json', 'X-RN-Inline': 'true'}, body});
        if (!response.ok) throw new Error((await response.text()).trim());
        const result = await response.json();
        // HTML comes from the server's autoescaped directory-row template.
        const table = document.createElement('table');
        table.innerHTML = `<tbody>${result.Row}</tbody>`;
        const row = table.querySelector('tr[data-path]');
        if (!row) throw new Error('Link saved. Reload the directory to see it.');
        other.append(row);
        refreshRanking();
        row.classList.add('new-link-row');
        row.scrollIntoView({block: 'nearest', behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'});
        row.addEventListener('animationend', () => row.classList.remove('new-link-row'), {once: true});
        short.value = destination.value = description.value = '';
        checked = '';
        serverError = '';
        message(`Added ${result.Short}/`);
        short.focus({preventScroll: true});
      } catch (error) {
        message(error.message || 'Could not confirm the save. Check the directory before retrying.', true);
      } finally {
        saving = false;
        save.textContent = 'Save link ↗';
        [short, destination, description].forEach(input => input.readOnly = false);
        update();
      }
    });
    window.addEventListener('pageshow', validate);
    update();
    if (short.value && destination.value) validate();
  }
})();
