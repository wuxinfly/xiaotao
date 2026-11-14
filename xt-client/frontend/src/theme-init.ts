try {
  document.documentElement.dataset.theme = localStorage.getItem('xiaotao.theme') === 'light' ? 'light' : 'dark';
} catch {
  document.documentElement.dataset.theme = 'dark';
}
