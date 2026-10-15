(() => {
  const api = '/api';
  let active = null;

  function request(path, options = {}) {
    const token = localStorage.getItem('auth_token') || sessionStorage.getItem('auth_token');
    return fetch(api + path, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(options.headers || {}),
      },
    }).then(async (response) => {
      const body = await response.json();
      if (!response.ok || body.code !== 200) throw new Error(body.message || '请求失败');
      return body.data;
    });
  }

  function close() {
    if (active) active.remove();
    active = null;
  }

  async function showLogin(accountId) {
    close();
    const root = document.createElement('div');
    root.className = 'qms-115-login-overlay';
    root.innerHTML = `<section class="qms-115-login-dialog" role="dialog" aria-modal="true">
      <button class="qms-115-close" aria-label="关闭">×</button>
      <h2>115 扫码登录</h2>
      <p class="qms-115-hint">请选择一个未被日常设备使用的会话槽位。不会默认使用 iOS。</p>
      <label>登录设备 <select class="qms-115-app"><option>正在加载…</option></select></label>
      <div><button class="qms-115-start" disabled>生成二维码</button></div>
      <img class="qms-115-qr" alt="115 登录二维码">
      <p class="qms-115-status">请选择登录设备后生成二维码</p>
    </section>`;
    document.body.append(root);
    active = root;
    const app = root.querySelector('.qms-115-app');
    const start = root.querySelector('.qms-115-start');
    const qr = root.querySelector('.qms-115-qr');
    const status = root.querySelector('.qms-115-status');
    let timer = null;
    const stop = () => { if (timer) clearInterval(timer); timer = null; };
    root.querySelector('.qms-115-close').onclick = () => { stop(); close(); };
    root.onclick = (event) => { if (event.target === root) { stop(); close(); } };

    try {
      const apps = await request('/115/login-apps');
      app.replaceChildren(...apps.map((item) => {
        const option = document.createElement('option');
        option.value = item.app;
        option.textContent = `${item.description} (${item.app})`;
        return option;
      }));
      start.disabled = false;
    } catch (error) {
      status.textContent = `设备列表加载失败：${error.message}`;
      return;
    }

    start.onclick = async () => {
      stop();
      start.disabled = true;
      app.disabled = true;
      qr.removeAttribute('src');
      status.textContent = '正在获取二维码…';
      try {
        const data = await request('/auth/115-qrcode-open', {
          method: 'POST', body: JSON.stringify({ account_id: accountId, login_app: app.value }),
        });
        qr.src = data.qrcode;
        status.textContent = '请在官方 115 App 中扫码并确认';
        timer = setInterval(async () => {
          try {
            const result = await request('/auth/115-qrcode-status', {
              method: 'POST', body: JSON.stringify({ account_id: accountId, uid: data.uid }),
            });
            if (result.reason) throw new Error(result.reason);
            if (result.status === 1) status.textContent = '已扫码，请在 115 App 中确认';
            if (result.status === 2) {
              stop();
              status.textContent = '登录成功';
              setTimeout(() => { close(); location.reload(); }, 700);
            }
            if (result.status === -1 || result.status === -2) throw new Error('二维码已失效或取消');
          } catch (error) {
            stop(); start.disabled = false; app.disabled = false;
            status.textContent = error.message;
          }
        }, 1500);
      } catch (error) {
        start.disabled = false; app.disabled = false;
        status.textContent = `获取二维码失败：${error.message}`;
      }
    };
  }

  document.addEventListener('click', (event) => {
    const button = event.target.closest('button');
    if (!button || button.textContent.trim() !== '授权') return;
    const card = button.closest('.account-card');
    if (!card || !card.textContent.includes('115')) return;
    const match = card.textContent.match(/#(\d+)/);
    if (!match) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    showLogin(Number(match[1]));
  }, true);
})();

