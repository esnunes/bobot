htmx.on('bobot:redirect', e => {
  htmx.ajax('GET', e.detail.path, {target: 'body', swap: 'innerHTML'});
})
