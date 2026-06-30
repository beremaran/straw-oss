// Straw Management UI Global State Store

const subscribers = [];

export const state = {
  baseUrl: localStorage.getItem('straw_baseUrl') || 'http://localhost:8081',
  token: localStorage.getItem('straw_token') || '',
  remember: localStorage.getItem('straw_remember') === 'true',
  currentPage: window.location.hash || '#/overview',
  isLoading: false,
  error: null,
  toast: null, // { message, type: 'success' | 'error' | 'info' }
  confirmDialog: null, // { title, body, confirmText, callback, requiresInput, inputVal, loading }
  // Cached page states
  overviewData: null,
  apiKeysData: null,
  rulesData: null,
  endpointsData: null,
  fingerprintsData: null,
  usageData: null,
  cacheData: null,
  systemData: null
};

export function setState(changes) {
  Object.assign(state, changes);
  subscribers.forEach(cb => cb(state));
}

export function subscribe(cb) {
  subscribers.push(cb);
  return () => {
    const idx = subscribers.indexOf(cb);
    if (idx !== -1) subscribers.splice(idx, 1);
  };
}

export function showToast(message, type = 'success') {
  setState({ toast: { message, type } });
  setTimeout(() => {
    if (state.toast && state.toast.message === message) {
      setState({ toast: null });
    }
  }, 4000);
}

export function showConfirm(options) {
  setState({
    confirmDialog: {
      ...options,
      inputVal: '',
      loading: false
    }
  });
}

export function closeConfirm() {
  setState({ confirmDialog: null });
}

export function clearSession() {
  setState({
    token: '',
    overviewData: null,
    apiKeysData: null,
    rulesData: null,
    endpointsData: null,
    fingerprintsData: null,
    usageData: null,
    cacheData: null,
    systemData: null
  });
  if (state.remember) {
    localStorage.removeItem('straw_token');
  } else {
    localStorage.removeItem('straw_token');
    localStorage.removeItem('straw_baseUrl');
    localStorage.removeItem('straw_remember');
  }
}
