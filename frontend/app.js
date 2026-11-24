const $=id=>document.getElementById(id);
const api=()=>window.go?.gui?.App;
let busy=false,lastState='',activePage='status',rememberTouched=false;
const labels={STARTING:'正在启动',AUTH_REQUIRED:'需要登录',AUTHENTICATING:'正在登录',READY:'已连接',ERROR:'连接异常',STOPPED:'代理未运行'};
function notice(message){$('notice').textContent=String(message||'');$('notice').hidden=!message;}
function page(name){if(name!=='account')hidePassword();activePage=name;document.querySelectorAll('.page').forEach(el=>el.hidden=el.id!==name);document.querySelectorAll('nav button').forEach(el=>el.classList.toggle('selected',el.dataset.page===name));$('page-title').textContent=document.querySelector('nav button[data-page="'+name+'"]').textContent.trim().slice(1).trim();if(name==='logs')loadLogs();}
document.querySelectorAll('nav button').forEach(b=>b.onclick=()=>page(b.dataset.page));
$('open-web-terminal').onclick=()=>api().OpenWebTerminal();
function render(s){
const forbidden=s.error_code==='WEBSHELL_FORBIDDEN';
$('open-web-terminal').hidden=!forbidden;
lastState=s.state;$('state').textContent=labels[s.state]||s.state;$('status-dot').classList.toggle('ready',s.state==='READY');$('account-value').textContent=s.account||'尚未登录';$('node-value').textContent=s.node||'—';$('address-value').textContent=s.address||'—';$('command').textContent='ssh '+(s.alias||'seusc');$('state-description').textContent=s.state==='READY'?'连接已就绪，现在可以打开终端开始工作。':s.state==='AUTHENTICATING'?'正在获取登录会话。若出现浏览器窗口，请完成所需验证。':forbidden?'官方 WebSSH 页面拒绝了终端连接（HTTP 403），自动重试已停止。':s.error_code==='NODE_UNAVAILABLE'?'请检查默认节点是否与官方终端列表中的名称和 ID 一致。':s.state==='ERROR'?'连接暂时失败，后台会自动重试。':'启动后台代理并登录 SEU 账号，建立你的本地 SSH 入口。';$('version').textContent='v'+(s.version||'unknown');$('update-current').textContent='v'+(s.version||'unknown');$('start').hidden=s.state!=='STOPPED';$('restart').hidden=s.state==='STOPPED';$('stop').hidden=s.state==='STOPPED';if(document.activeElement!==$('username')&&!$('username').value)$('username').value=s.account||'';if(!busy){$('autostart').checked=!!s.autostart;$('login-autostart').checked=s.autostart!==false;if(!rememberTouched)$('remember').checked=!!s.remember;}updateSavedAccount(s);
const nodes=Object.entries(s.nodes||{});if(JSON.stringify(nodes)!==$('nodes').dataset.nodes){$('nodes').replaceChildren(...nodes.map(([name,id])=>{const o=document.createElement('option');o.value=name;o.textContent=name+' · '+id;return o;}));$('nodes').dataset.nodes=JSON.stringify(nodes);} $('nodes').value=s.node||'';}
async function refresh(){if(!api()){notice('请在 SEU SC Bridge 桌面应用中打开此界面。');render({state:'STOPPED'});return;}try{render(await api().Status());}catch(e){render({state:'STOPPED'});}}
async function action(fn){if(busy)return;busy=true;notice('');document.querySelectorAll('button:not(nav button)').forEach(b=>b.disabled=true);try{await fn();}catch(e){notice(e);}finally{busy=false;document.querySelectorAll('button').forEach(b=>b.disabled=false);await refresh();}}
const passwordField = $('password');
const revealButton = $('toggle-password');
const emptyHint = passwordField.placeholder;
let savedAccount = '', fromStore = false, revealRequest = 0;

function hasSavedPassword() {
  return !!savedAccount && savedAccount === $('username').value.trim();
}
function renderPasswordHint() {
  const saved = hasSavedPassword();
  passwordField.placeholder = saved && $('remember').checked ? '••••••••' : emptyHint;
  const hint = saved ? ($('remember').checked ? '此账号的密码已保存在系统凭据管理器中。' : '下次登录时将清除此账号已保存的密码。')
    : ($('remember').checked ? '登录成功后将记住输入的密码。' : '当前不会保存密码。');
  if ($('credential-status').textContent !== hint) $('credential-status').textContent = hint;
}
function updateSavedAccount(status) {
  const account = status.remember && status.credential_stored ? status.account : '';
  if (account !== savedAccount) hidePassword();
  savedAccount = account;
  renderPasswordHint();
}
function hidePassword() {
  revealRequest++;
  passwordField.type = 'password';
  revealButton.textContent = '显示密码';
  if (fromStore) passwordField.value = '';
  fromStore = false;
}
revealButton.onclick = async () => {
  if (passwordField.type === 'text') { hidePassword(); return; }
  const request = ++revealRequest;
  if (!passwordField.value && hasSavedPassword() && $('remember').checked) {
    const account = savedAccount;
    try {
      const password = await api().RevealSavedPassword(account);
      if (request !== revealRequest || !hasSavedPassword() || account !== savedAccount || passwordField.value) return;
      passwordField.value = password;
      fromStore = true;
    } catch (err) {
      if (request === revealRequest) notice(err);
      return;
    }
  }
  passwordField.type = 'text';
  revealButton.textContent = '隐藏密码';
};
passwordField.oninput = () => { revealRequest++; fromStore = false; };
$('username').oninput = () => { hidePassword(); passwordField.value = ''; renderPasswordHint(); };
$('remember').onchange = () => { rememberTouched = true; hidePassword(); renderPasswordHint(); };
window.addEventListener('blur', hidePassword);
document.addEventListener('visibilitychange', () => { if (document.hidden) hidePassword(); });
$('login-form').onsubmit = e => {
  e.preventDefault();
  if (busy) return;
  const password = fromStore ? '' : passwordField.value;
  hidePassword();
  passwordField.value = '';
  action(async () => {
    notice('正在登录，请稍候…');
    await api().Login($('username').value.trim(),password,$('remember').checked,$('login-autostart').checked);
    rememberTouched = false;
    notice('');
    page('status');
  });
};
$('check-updates').onclick=()=>checkUpdates(false);
$('install-update').onclick=()=>action(async()=>{notice('正在下载并验证安装文件…');await api().InstallUpdate();notice('更新安装程序已启动，请按向导完成更新。');});
$('start').onclick=()=>action(()=>api().Start());$('restart').onclick=()=>action(()=>api().Restart());$('stop').onclick=()=>action(()=>api().Stop());$('logout').onclick=()=>action(async()=>{hidePassword();passwordField.value='';await api().Logout();rememberTouched=false;});$('copy').onclick=()=>action(async()=>{await api().CopyCommand();notice('已复制 SSH 命令。');});$('nodes').onchange=()=>action(()=>api().SetNode($('nodes').value,0));$('node-form').onsubmit=e=>{e.preventDefault();action(()=>api().SetNode($('new-node').value.trim(),Number($('new-node-id').value)));};$('autostart').onchange=()=>action(()=>api().SetAutoStart($('autostart').checked));async function loadLogs(){if(!api())return;try{$('log-output').textContent=await api().Logs()||'暂无日志';}catch(e){$('log-output').textContent=String(e);}}$('refresh-logs').onclick=loadLogs;
async function checkUpdates(silent){if(!api())return;$('check-updates').disabled=true;$('update-message').textContent='正在检查 GitHub Releases…';try{const info=await api().CheckForUpdates();$('update-current').textContent='v'+info.currentVersion;$('update-message').textContent=info.updateAvailable?'发现新版本 v'+info.latestVersion+'。':('当前已是最新版本 v'+info.currentVersion+'。');$('install-update').hidden=!info.updateAvailable||!info.canInstall;$('release-link').hidden=!info.releaseUrl;$('release-link').href=info.releaseUrl||'#';$('update-notes').textContent=info.notes||'';$('update-notes').hidden=!info.notes;}catch(e){$('update-message').textContent=silent?'暂时无法检查更新，请稍后重试。':'检查更新失败：'+String(e);$('install-update').hidden=true;}finally{$('check-updates').disabled=false;}}
refresh();checkUpdates(true);setInterval(refresh,2500);
