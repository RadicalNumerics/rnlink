(() => {
  const form = document.getElementById('create-form');
  if (!form) return;
  const short = form.elements.short;
  const destination = form.elements.long;
  const description = form.elements.description;
  const fields = document.getElementById('create-fields');
  const prefix = document.getElementById('path-prefix').textContent;
  function update() {
    const expanded = [short, destination, description].some(input => input.value.length > 0);
    fields.hidden = !expanded;
    document.getElementById('create-link').classList.toggle('is-collapsed', !expanded);
    description.setCustomValidity(description.value.trim() ? '' : 'Please add a description.');
    document.getElementById('preview-path').textContent = prefix + short.value.trim() + '/';
    document.getElementById('preview-description').textContent = description.value.trim() || 'Describe this link';
    document.getElementById('preview-destination').textContent = destination.value.trim() || 'Add a destination';
  }
  form.addEventListener('input', update);
  window.addEventListener('pageshow', update);
  update();
})();
