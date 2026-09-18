(() => {
  'use strict';

  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];
  const state = { root: '/data/adb/modules/yumi_jooo', dataDir: '/data/adb/yumi_jooo', status: null, page: 'overview', refreshing: false, timer: null, games: [], onboardingShown: false };
  const callbacks = window.__JoooCallbacks = {};
  let cbSeq = 0;
  let toastTimer = 0;

  function shellQuote(v) { return `'${String(v).replace(/'/g, `'\\''`)}'`; }
  function hasKsu() { return typeof window.ksu !== 'undefined' && typeof window.ksu.exec === 'function'; }

  function toast(msg) {
    const el = $('#toast');
    el.textContent = String(msg);
    el.classList.add('show');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.remove('show'), 2300);
    try { if (window.ksu && ksu.toast) ksu.toast(String(msg)); } catch (_) {}
  }

  function execAsync(cmd) {
    return new Promise((resolve, reject) => {
      if (!hasKsu()) return reject(new Error('当前不是 KernelSU WebUI 环境'));
      const id = `cb${++cbSeq}`;
      callbacks[id] = (code, stdout, stderr) => {
        delete callbacks[id];
        if (Number(code) === 0) resolve(String(stdout || ''));
        else reject(new Error(String(stderr || stdout || `命令失败(${code})`)));
      };
      try { ksu.exec(cmd, `window.__JoooCallbacks.${id}`); }
      catch (e) { delete callbacks[id]; reject(e); }
    });
  }

  function jood(args) {
    const cmd = [shellQuote(`${state.root}/bin/jood`), ...args.map(shellQuote)].join(' ');
    return execAsync(cmd);
  }

  function initModuleRoot() {
    try {
      if (window.ksu && ksu.moduleInfo) {
        const info = JSON.parse(ksu.moduleInfo());
        if (info && info.moduleDir) state.root = info.moduleDir;
      }
    } catch (_) {}
  }

  function fmtGHz(khz) {
    const n = Number(khz || 0);
    return n > 0 ? `${(n / 1e6).toFixed(n >= 1e6 ? 2 : 3)}` : '--';
  }
  function pct(v) { return `${Math.round(Math.max(0, Math.min(1, Number(v || 0))) * 100)}`; }
  function tierName(t) { return ({powersave:'省电',balance:'均衡',performance:'性能',fast:'极速'})[t] || t || '--'; }
  function logName(n) { return ['精简','标准','详细'][Number(n)] || '标准'; }
  function controlModeName(m) { return ({auto:'自动识别',scene:'Scene',webui:'WebUI手动'})[m] || m || '--'; }
  function profileName(p) { return p === 'extreme' ? '极致游戏' : '均衡游戏'; }
  function loadName(l) { return ({light:'轻载 · 自动降一档',normal:'正常负载',heavy:'重载 · 自动升一档'})[l] || '正常负载'; }
  function esc(s) { return String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }

  async function refreshStatus(force = false) {
    if (state.refreshing && !force) return;
    state.refreshing = true;
    $('#refreshBtn').classList.add('rotating');
    try {
      const raw = await jood(['status']);
      const st = JSON.parse(raw);
      state.status = st;
      renderStatus(st);
      renderConfig(st.config || {});
      maybeShowOnboarding(st);
    } catch (e) {
      $('#strategyTitle').textContent = '后端连接失败';
      $('#strategyDesc').textContent = e.message || String(e);
      $('#runtimeBadge').className = 'status-badge';
      $('#runtimeBadge').innerHTML = '<span></span>异常';
    } finally {
      state.refreshing = false;
      $('#refreshBtn').classList.remove('rotating');
    }
  }

  function renderStatus(st) {
    const game = !!(st.daemon && st.daemon.running);
    $('#strategyTitle').textContent = game ? '游戏动态控制' : '原厂低功耗日常';
    $('#strategyDesc').textContent = game
      ? 'jood 只抬必要的 CPU 最低频率，原厂 CPU 调速器 继续负责瞬态调频。'
      : 'Jooo 后端不常驻接管 CPU，日常完整交回 OEM 原厂 CPU 调速器。';
    const badge = $('#runtimeBadge');
    badge.className = `status-badge ${game ? 'game' : 'good'}`;
    badge.innerHTML = `<span></span>${game ? '游戏引擎运行中' : '日常低功耗'}`;
    const cm = st.config?.control_mode || 'auto';
    $('#sceneTag').textContent = `${controlModeName(cm)} · ${tierName(st.scene_mode)}`;
    if (!game) {
      if (cm === 'auto') $('#strategyDesc').textContent = st.watch?.running ? 'jood低频监听前台应用；日常不写CPU频率，命中游戏才启动控制器。' : '自动监听未运行；当前仍保持OEM原厂CPU调速器。';
      if (cm === 'scene') $('#strategyDesc').textContent = 'Scene 9.4.18 负责场景切换；Jooo只旁路接收事件，不覆盖Scene原始统计。';
      if (cm === 'webui') $('#strategyDesc').textContent = '不会自动切换；只有WebUI手动命令才能启动或停止游戏控制器。';
    }
    $('#packageTag').textContent = st.package ? `前台 ${st.package}` : '前台 --';
    $('#versionTag').textContent = st.version || 'v--';
    $('#powerValue').textContent = Number(st.power_w) > 0 ? Number(st.power_w).toFixed(2) : '--';
    $('#tempValue').textContent = Number(st.battery_temp_c) > 0 ? Number(st.battery_temp_c).toFixed(1) : '--';
    state.dataDir = st.data_dir || '/data/adb/yumi_jooo';
    $('#demandValue').textContent = game ? pct(st.daemon.demand) : '--';
    $('#sampleValue').textContent = st.config?.sample_ms ?? '--';
    const daily = st.daily || {};
    $('#dailyOptState').textContent = daily.enabled ? (daily.applied ? '已应用' : '待应用/无需写入') : '已关闭';
    $('#dailyEffCpus').textContent = daily.efficiency_cpus || '--';
    $('#dailyCpuset').textContent = daily.cpuset_path || '未写入';
    $('#dailyZram').textContent = daily.zram_status || '--';

    const grid = $('#clusterGrid');
    grid.innerHTML = '';
    const roleNames = {efficiency:'能效簇',performance_primary:'主性能簇',performance_secondary:'次性能簇',prime:'Prime'};
    for (const p of (st.policies || [])) {
      const min = Number(p.min_khz || 0), max = Number(p.max_khz || 0), cur = Number(p.cur_khz || 0);
      const prog = max > min ? Math.max(0, Math.min(100, (cur - min) / (max - min) * 100)) : 0;
      const card = document.createElement('article');
      card.className = 'cluster-card glass';
      card.innerHTML = `<div class="cluster-top"><div><div class="cluster-name">${esc(roleNames[p.role] || `policy${p.id}`)}</div><div class="cluster-cpus">policy${p.id} · CPU ${esc(p.cpus)} · cap ${esc(p.capacity || '--')}</div></div><div class="cluster-freq">${fmtGHz(cur)}<small>GHz</small></div></div><div class="freq-bar"><div class="freq-fill" style="width:${prog.toFixed(1)}%"></div></div><div class="cluster-meta"><span>${fmtGHz(min)}G 最低</span><span>${esc(p.governor || '--')}</span><span>${fmtGHz(max)}G 最高</span></div>`;
      grid.appendChild(card);
    }

    $('#gameRuntimeCard').classList.toggle('hidden', !game);
    if (game) {
      $('#daemonPackage').textContent = st.daemon.package || '--';
      $('#daemonTier').textContent = st.daemon.thermal_limited ? `${tierName(st.daemon.tier)} → ${tierName(st.daemon.effective_tier || 'balance')}` : `${tierName(st.daemon.tier)} → ${tierName(st.daemon.effective_tier || st.daemon.tier)}`;
      $('#daemonProfile').textContent = profileName(st.daemon.game_profile);
      $('#loadLevelState').textContent = loadName(st.daemon.load_level);
      $('#effectivePrimeGate').textContent = Number(st.daemon.effective_prime_gate || 0) > 0 ? `${Math.round(Number(st.daemon.effective_prime_gate)*100)}%` : '--';
      $('#topThread').textContent = st.daemon.top_thread ? `${st.daemon.top_thread} · ${(Number(st.daemon.top_util || 0)*100).toFixed(0)}%` : '--';
      $('#primeState').textContent = st.daemon.prime_open ? '已放行' : '关闭';
      $('#burstState').textContent = st.daemon.burst ? '已触发' : '空闲';
      $('#thermalState').textContent = st.daemon.thermal_limited ? `${Number(st.daemon.battery_temp_c || 0).toFixed(1)}°C · 已降至均衡` : `${Number(st.daemon.battery_temp_c || st.battery_temp_c || 0).toFixed(1)}°C · 正常`;
    }

    const historyBox = $('#historyList');
    if (historyBox) {
      const items = Array.isArray(st.history) ? st.history.slice(0, 6) : [];
      historyBox.innerHTML = '';
      if (!items.length) {
        historyBox.innerHTML = '<div class="history-empty">暂无切换记录</div>';
      } else {
        for (const h of items) {
          const row = document.createElement('div');
          row.className = 'history-row';
          const isGame = h.category === 'game';
          row.innerHTML = `<span class="history-dot ${isGame ? 'game' : ''}"></span><div class="history-main"><strong>${isGame ? '游戏' : '日常'} · ${esc(tierName(h.mode))}</strong><small>${esc(h.package || '系统/桌面')}</small></div><time>${esc(h.time || '--')}</time>`;
          historyBox.appendChild(row);
        }
      }
    }
    renderForeground(st);
  }

  function appInfo(pkg) {
    if (!pkg) return null;
    try {
      if (window.ksu && ksu.getPackagesInfo) {
        const arr = JSON.parse(ksu.getPackagesInfo(JSON.stringify([pkg])));
        if (Array.isArray(arr) && arr[0] && !arr[0].error) return arr[0];
      }
    } catch (_) {}
    return { packageName: pkg, appLabel: pkg };
  }

  function renderForeground(st) {
    const pkg = st.foreground_package || '';
    const info = appInfo(pkg);
    $('#foregroundPkg').textContent = pkg || '--';
    $('#foregroundName').textContent = info?.appLabel || (pkg ? '当前前台应用' : '未读取到前台应用');
    const icon = $('#foregroundIcon');
    icon.innerHTML = '';
    if (pkg) {
      const img = document.createElement('img');
      img.src = `ksu://icon/${encodeURIComponent(pkg)}`; img.alt = '';
      img.onerror = () => { icon.textContent = (info?.appLabel || pkg).slice(0,1); };
      icon.appendChild(img);
    } else icon.textContent = '?';
    const known = state.games.some(g => g.package === pkg) || (st.config?.custom_games || []).includes(pkg);
    $('#addForegroundBtn').disabled = !pkg || known;
    $('#addForegroundBtn').textContent = known ? '已在列表' : '加入游戏';

    const cap = st.captured_package || '';
    $('#capturedPkg').textContent = cap || '--';
    $('#addCapturedBtn').classList.toggle('hidden', !cap);
    $('#addCapturedBtn').disabled = !cap || state.games.some(g => g.package === cap) || (st.config?.custom_games || []).includes(cap);
  }

  function updatePrimeGateHint(v) {
    v = Number(v);
    const extreme = Math.max(50, v - 22);
    const text = v <= 68 ? '均衡档偏性能；X4更容易参与。' : v <= 75 ? '均衡游戏推荐区间；多数SM8650建议先从70–75%测试。' : v <= 85 ? '均衡档偏省电；Prime只在更重负载时开放。' : '非常保守：可能降低重负载游戏的瞬时响应。';
    $('#primeGateHint').textContent = `${text} 当前极致游戏派生阈值约 ${extreme}%。`;
  }

  function renderConfig(c) {
    const cm = c.control_mode || 'auto';
    $('#controlMode').value = cm;
    $('#sceneEnabled').checked = !!c.scene_enabled;
    $('#watchEnabled').checked = c.watch_enabled !== false;
    $('#gameEngine').checked = !!c.game_engine;
    $('#adaptiveGameLoad').checked = c.adaptive_game_load !== false;
    $('#dailyOptimize').checked = c.daily_opt_enabled !== false;
    $('#dailyBgLimit').checked = c.daily_bg_limit !== false;
    $('#zramPreferZstd').checked = c.zram_prefer_zstd !== false;
    $('#dailyBgLimit').disabled = c.daily_opt_enabled === false;
    $('#zramPreferZstd').disabled = c.daily_opt_enabled === false;
    $('#followSceneTier').checked = !!c.follow_scene_tier;
    $('#followSceneTier').disabled = cm !== 'scene';
    $('#watchEnabledRow').classList.toggle('hidden', cm !== 'auto');
    $('#watchIntervalCard').classList.toggle('hidden', cm !== 'auto');
    $('#manualControlCard').classList.toggle('hidden', cm !== 'webui');
    const hints = {
      auto:'jood每隔一段时间识别前台APP；无需Scene。日常不写CPU频率，命中游戏才启动控制器。',
      scene:'仅接受Scene 9.4.18包装层事件；Scene原powercfg.sh会继续执行，powercfg.json保持不动。',
      webui:'关闭自动/Scene控制，只接受WebUI手动切换，适合测试与排障。'
    };
    $('#controlModeHint').textContent = hints[cm] || '';
    $('#threadPlacement').checked = !!c.thread_placement;
    $('#thermalGuard').checked = c.thermal_guard !== false;
    $('#thermalLimitCard').classList.toggle('hidden', c.thermal_guard === false);
    $('#fixedTierCard').classList.toggle('hidden', cm === 'scene' && !!c.follow_scene_tier);
    const gp = c.game_profile_default || 'balanced';
    $('#gameProfileText').textContent = profileName(gp);
    $$('#gameProfileSegments button').forEach(b => b.classList.toggle('active', b.dataset.profile === gp));
    $('#fixedTierText').textContent = tierName(c.fixed_game_tier);
    $$('#tierSegments button').forEach(b => b.classList.toggle('active', b.dataset.tier === c.fixed_game_tier));
    const pg = Math.round(Number(c.prime_gate || .72) * 100);
    $('#primeGate').value = pg; $('#primeGateText').textContent = pg; updatePrimeGateHint(pg);
    const tl = Number(c.battery_temp_limit_c || 45); $('#thermalLimit').value = tl; $('#thermalLimitText').textContent = tl.toFixed(tl % 1 ? 1 : 0);
    const wi = Number(c.watch_interval_ms || 2000); $('#watchInterval').value = wi; $('#watchIntervalText').textContent = wi;
    const sm = Number(c.sample_ms || 120); $('#sampleMS').value = sm; $('#sampleText').textContent = sm;
    $('#logLevelText').textContent = logName(c.log_level);
    $$('#logSegments button').forEach(b => b.classList.toggle('active', Number(b.dataset.log) === Number(c.log_level)));
    renderCustomGames(c.custom_games || []);
  }

  async function setConfig(key, value, silent = false) {
    try {
      await jood(['config','set',key,String(value)]);
      if (!silent) toast('已生效');
      await refreshStatus(true);
    } catch (e) { toast(`修改失败：${e.message}`); await refreshStatus(true); }
  }

  async function loadGames() {
    $('#gamesList').innerHTML = '<div class="empty glass">正在检测已安装游戏…</div>';
    try {
      const games = JSON.parse(await jood(['games']));
      state.games = Array.isArray(games) ? games : [];
      if (state.status) renderForeground(state.status);
      $('#gameCount').textContent = `已检测到 ${state.games.length} 个游戏`;
      const box = $('#gamesList'); box.innerHTML = '';
      if (!state.games.length) box.innerHTML = '<div class="empty glass">未检测到已知/自定义游戏。可以在下方手动添加。</div>';
      for (const g of state.games) {
        const item = document.createElement('div'); item.className = 'game-item glass';
        const rec = g.recommended_profile || 'balanced';
        const profile = g.profile || state.status?.config?.game_profile_default || 'balanced';
        item.innerHTML = `<div class="game-avatar"><img src="ksu://icon/${encodeURIComponent(g.package)}" alt="" onerror="this.style.display='none';this.parentNode.textContent='${esc((g.name || g.package || '?').slice(0,1))}'"></div><div class="game-info"><div class="game-name">${esc(g.name || '游戏')}<span class="recommend-pill">推荐${profileName(rec)}</span></div><div class="game-pkg">${esc(g.package)}</div></div><span class="badge-mini">${g.custom ? '自定义' : '自动识别'}</span><div class="game-profile-control"><span>游戏类型</span><div class="mini-segments"><button data-game-profile="balanced" class="${profile==='balanced'?'active':''}">均衡</button><button data-game-profile="extreme" class="${profile==='extreme'?'active':''}">极致</button></div></div>`;
        item.querySelectorAll('[data-game-profile]').forEach(b => b.onclick = () => setGameProfile(g.package, b.dataset.gameProfile));
        box.appendChild(item);
      }
    } catch (e) {
      $('#gamesList').innerHTML = `<div class="empty glass">检测失败：${esc(e.message)}</div>`;
      $('#gameCount').textContent = '游戏检测失败';
    }
  }

  function renderCustomGames(list) {
    const box = $('#customGamesList'); box.innerHTML = '';
    if (!list.length) return;
    for (const pkg of list) {
      const item = document.createElement('div'); item.className='game-item glass';
      item.innerHTML = `<div class="game-avatar">+</div><div class="game-info"><div class="game-name">自定义游戏</div><div class="game-pkg">${esc(pkg)}</div></div><button class="remove-btn" data-remove-game="${esc(pkg)}">移除</button>`;
      box.appendChild(item);
    }
  }

  async function addCustomGame(pkg) {
    pkg = String(pkg || '').trim();
    if (!/^[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)+$/.test(pkg)) return toast('包名格式不正确');
    try { await jood(['config','add-game',pkg]); $('#customGameInput').value=''; toast('已添加游戏'); await refreshStatus(true); await loadGames(); }
    catch (e) { toast(`添加失败：${e.message}`); }
  }

  async function removeCustomGame(pkg) {
    try { await jood(['config','remove-game',pkg]); toast('已移除'); await refreshStatus(true); await loadGames(); }
    catch (e) { toast(`移除失败：${e.message}`); }
  }

  async function setGameProfile(pkg, profile) {
    try {
      await jood(['config','set-game-profile',pkg,profile]);
      toast(`${pkg} → ${profileName(profile)}`);
      await refreshStatus(true);
      await loadGames();
    } catch (e) { toast(`游戏类型修改失败：${e.message}`); }
  }

  function maybeShowOnboarding(st) {
    if (state.onboardingShown || st.config?.onboarding_done !== false) return;
    state.onboardingShown = true;
    const draft = { step: 1, control: st.scene_installed ? 'scene' : 'auto', profile: 'balanced' };
    const render = () => {
      const wrap = document.createElement('div'); wrap.className = 'onboard';
      wrap.innerHTML = `<div class="onboard-progress"><i class="${draft.step>=1?'active':''}"></i><i class="${draft.step>=2?'active':''}"></i><i class="${draft.step>=3?'active':''}"></i></div>`;
      const card = document.createElement('div'); card.className = 'onboard-card'; wrap.appendChild(card);
      if (draft.step === 1) {
        const ok = !!st.scene_installed;
        card.innerHTML = `<strong>第 1 步 · 检测 Scene</strong><div class="scene-detect ${ok?'ok':''}"><span class="dot"></span><span>${ok?'已检测到 Scene（com.omarea.vtools）':'未检测到 Scene'}</span></div><p style="margin-top:9px">装了 Scene 推荐由 Scene 管场景；没装也没关系，jood 可以独立自动识别前台游戏。</p>`;
        openModal('首次安装引导 · 1/3', wrap, [{text:'下一步',className:'primary',close:false,onClick:()=>{draft.step=2;render();}}]);
        return;
      }
      if (draft.step === 2) {
        card.innerHTML = `<strong>第 2 步 · 选择控制模式</strong><p>已根据当前环境预选推荐项。</p><div class="onboard-choice"><button data-ctl="scene"><b>Scene 接管</b><small>适合已经使用 Scene 的用户，保留 Scene 原统计和切档。</small></button><button data-ctl="auto"><b>jood 自动识别</b><small>无需 Scene，每 2 秒识别前台应用，命中游戏才启动控制器。</small></button><button data-ctl="webui"><b>WebUI 手动</b><small>只用于测试/排障，不自动切换。</small></button></div>`;
        card.querySelectorAll('[data-ctl]').forEach(b=>{b.classList.toggle('active',b.dataset.ctl===draft.control);b.onclick=()=>{draft.control=b.dataset.ctl;render();};});
        openModal('首次安装引导 · 2/3', wrap, [{text:'上一步',close:false,onClick:()=>{draft.step=1;render();}},{text:'下一步',className:'primary',close:false,onClick:()=>{draft.step=3;render();}}]);
        return;
      }
      card.innerHTML = `<strong>第 3 步 · 初始游戏类型</strong><p>建议先用均衡游戏。大型开放世界/重负载游戏可单独改成极致。</p><div class="onboard-choice"><button data-prof="balanced"><b>均衡游戏 · 推荐</b><small>Prime Gate 基准 72%，兼顾功耗、温度和流畅度。</small></button><button data-prof="extreme"><b>极致游戏</b><small>Prime Gate 默认约 50%，更适合原神/鸣潮等大型游戏，发热更高。</small></button></div>`;
      card.querySelectorAll('[data-prof]').forEach(b=>{b.classList.toggle('active',b.dataset.prof===draft.profile);b.onclick=()=>{draft.profile=b.dataset.prof;render();};});
      openModal('首次安装引导 · 3/3', wrap, [{text:'上一步',close:false,onClick:()=>{draft.step=2;render();}},{text:'完成初始化',className:'primary',onClick:async()=>{
        try {
          await jood(['config','set','control_mode',draft.control]);
          await jood(['config','set','fixed_game_tier','balance']);
          await jood(['config','set','game_profile_default',draft.profile]);
          await jood(['config','set','onboarding_done','true']);
          toast(`初始化完成：${controlModeName(draft.control)} · ${profileName(draft.profile)}`);
          await refreshStatus(true);
        } catch(e) { toast(`初始化失败：${e.message}`); }
      }}]);
    };
    render();
  }

  function openModal(title, body, actions = []) {
    $('#modalTitle').textContent = title;
    const bodyEl = $('#modalBody'); bodyEl.innerHTML = ''; if (typeof body === 'string') bodyEl.innerHTML = body; else if (body) bodyEl.appendChild(body);
    const actionsEl = $('#modalActions'); actionsEl.innerHTML = '';
    for (const a of actions) { const b=document.createElement('button'); b.textContent=a.text; if(a.className)b.className=a.className; b.onclick=async()=>{ if(a.close!==false)closeModal(); if(a.onClick)await a.onClick(); }; actionsEl.appendChild(b); }
    $('#modal').classList.add('open'); $('#modal').setAttribute('aria-hidden','false');
  }
  function closeModal(){ $('#modal').classList.remove('open'); $('#modal').setAttribute('aria-hidden','true'); }
  $$('[data-close-modal]').forEach(x => x.addEventListener('click', closeModal));

  async function runSelftest() {
    try {
      const r = JSON.parse(await jood(['selftest']));
      const box=document.createElement('div'); box.className='test-list';
      for(const c of (r.checks||[])){const row=document.createElement('div');row.className=`test-row ${c.ok?'ok':''}`;row.innerHTML=`<span class="test-dot"></span><div><strong>${esc(c.name)}</strong><small>${esc(c.detail)}</small></div>`;box.appendChild(row);}
      openModal(r.ok?'自检通过':'发现异常', box, [{text:'关闭',className:'primary'}]);
    } catch(e){toast(`自检失败：${e.message}`);}
  }

  async function reloadLogs() {
    $('#logView').textContent='正在读取…';
    try {
      const cmd = `for f in ${shellQuote(state.dataDir+'/logs/service.log')} ${shellQuote(state.dataDir+'/logs/scene.log')} ${shellQuote(state.dataDir+'/logs/jood.log')} ${shellQuote(state.dataDir+'/logs/jood.log.1')}; do [ -f "$f" ] && { echo "===== ${'$'}{f##*/} ====="; tail -n 120 "$f"; }; done`;
      const out=await execAsync(cmd); $('#logView').textContent=out.trim()||'暂无日志。';
    }catch(e){$('#logView').textContent=`读取失败：${e.message}`;}
  }

  function pickInstalledApp() {
    try {
      if (!window.ksu || !ksu.listPackages) return toast('当前 KernelSU 不支持应用列表 API');
      const pkgs = JSON.parse(ksu.listPackages('user'));
      let infoMap = new Map();
      try {
        if (ksu.getPackagesInfo) {
          const info = JSON.parse(ksu.getPackagesInfo(JSON.stringify(pkgs)));
          infoMap = new Map(info.filter(x => !x.error).map(x => [x.packageName, x]));
        }
      } catch (_) {}

      const wrap=document.createElement('div'); wrap.className='app-picker';
      const search=document.createElement('input'); search.className='app-picker-search'; search.placeholder='搜索应用名称或包名';
      const list=document.createElement('div'); list.className='app-picker-list'; wrap.append(search,list);
      const render=(q='')=>{
        list.innerHTML='';
        const lower=q.trim().toLowerCase();
        const filtered=pkgs.filter(p=>{
          const label=String(infoMap.get(p)?.appLabel||'').toLowerCase();
          return !lower||p.toLowerCase().includes(lower)||label.includes(lower);
        }).slice(0,80);
        for(const p of filtered){
          const info=infoMap.get(p)||{};
          const b=document.createElement('button'); b.className='app-choice';
          b.innerHTML=`<img src="ksu://icon/${encodeURIComponent(p)}" alt=""><span><strong>${esc(info.appLabel||p)}</strong><small>${esc(p)}</small></span>`;
          b.onclick=()=>{$('#customGameInput').value=p;closeModal();};
          list.appendChild(b);
        }
        if(!filtered.length) list.innerHTML='<div class="history-empty">没有匹配的应用</div>';
      };
      search.oninput=()=>render(search.value); render(); openModal('选择已安装应用',wrap,[{text:'取消'}]); setTimeout(()=>search.focus(),150);
    } catch(e){toast(`读取应用失败：${e.message}`);}
  }

  function switchPage(name) {
    state.page=name;
    $$('.page').forEach(p=>p.classList.toggle('active',p.dataset.page===name));
    $$('.nav-btn').forEach(b=>b.classList.toggle('active',b.dataset.target===name));
    window.scrollTo({top:0,behavior:'auto'});
    if(name==='games'){ loadGames(); refreshStatus(true); }
    if(name==='diagnostics') reloadLogs();
    if(name==='overview') refreshStatus(true);
  }

  function bind() {
    $('#refreshBtn').onclick=()=>refreshStatus(true);
    $$('.nav-btn').forEach(b=>b.onclick=()=>switchPage(b.dataset.target));
    $('#controlMode').onchange=e=>setConfig('control_mode',e.target.value);
    $('#sceneEnabled').onchange=e=>setConfig('scene_enabled',e.target.checked);
    $('#watchEnabled').onchange=e=>setConfig('watch_enabled',e.target.checked);
    $('#gameEngine').onchange=e=>setConfig('game_engine',e.target.checked);
    $('#adaptiveGameLoad').onchange=e=>setConfig('adaptive_game_load',e.target.checked);
    $('#dailyOptimize').onchange=e=>setConfig('daily_opt_enabled',e.target.checked);
    $('#dailyBgLimit').onchange=e=>setConfig('daily_bg_limit',e.target.checked);
    $('#zramPreferZstd').onchange=e=>setConfig('zram_prefer_zstd',e.target.checked);
    $('#followSceneTier').onchange=e=>setConfig('follow_scene_tier',e.target.checked);
    $('#threadPlacement').onchange=e=>setConfig('thread_placement',e.target.checked);
    $('#thermalGuard').onchange=e=>setConfig('thermal_guard',e.target.checked);
    $$('#gameProfileSegments button').forEach(b=>b.onclick=()=>setConfig('game_profile_default',b.dataset.profile));
    $$('#tierSegments button').forEach(b=>b.onclick=()=>setConfig('fixed_game_tier',b.dataset.tier));
    $('#primeGate').oninput=e=>{ $('#primeGateText').textContent=e.target.value; updatePrimeGateHint(e.target.value); };
    $('#primeGate').onchange=e=>setConfig('prime_gate',(Number(e.target.value)/100).toFixed(2));
    $('#thermalLimit').oninput=e=>$('#thermalLimitText').textContent=Number(e.target.value).toFixed(Number(e.target.value)%1?1:0);
    $('#thermalLimit').onchange=e=>setConfig('battery_temp_limit_c',e.target.value);
    $('#watchInterval').oninput=e=>$('#watchIntervalText').textContent=e.target.value;
    $('#watchInterval').onchange=e=>setConfig('watch_interval_ms',e.target.value);
    $('#sampleMS').oninput=e=>$('#sampleText').textContent=e.target.value;
    $('#sampleMS').onchange=e=>setConfig('sample_ms',e.target.value);
    $$('#logSegments button').forEach(b=>b.onclick=()=>setConfig('log_level',b.dataset.log));
    $('#manualDailyBtn').onclick=async()=>{try{await jood(['manual','balance','app','']);toast('已切回日常均衡');await refreshStatus(true);}catch(e){toast(`手动切换失败：${e.message}`);}};
    $('#manualGameBtn').onclick=async()=>{
      const pkg=state.status?.foreground_package||'';
      if(!pkg)return toast('未读取到当前前台APP');
      const tier=state.status?.config?.fixed_game_tier||'balance';
      try{await jood(['manual',tier,'game',pkg]);toast(`已手动启动：${pkg}`);await refreshStatus(true);}catch(e){toast(`手动启动失败：${e.message}`);}
    };
    $('#resetConfigBtn').onclick=()=>openModal('恢复默认调校','这会恢复所有 Jooo 配置，但不会删除自定义模块文件。',[{text:'取消'},{text:'恢复默认',className:'primary',onClick:async()=>{try{await jood(['config','reset']);toast('已恢复默认');await refreshStatus(true);}catch(e){toast(e.message);}}}]);
    $('#reloadGamesBtn').onclick=loadGames;
    $('#addForegroundBtn').onclick=()=>addCustomGame(state.status?.foreground_package || '');
    $('#addCapturedBtn').onclick=()=>addCustomGame(state.status?.captured_package || '');
    $('#captureForegroundBtn').onclick=async()=>{
      toast('5秒后捕获：请立即切到目标游戏');
      try {
        const pkg=(await jood(['capture','5'])).trim();
        toast(`已捕获：${pkg}`);
        await refreshStatus(true);
      } catch(e) { toast(`捕获失败：${e.message}`); }
    };
    $('#addGameBtn').onclick=()=>addCustomGame($('#customGameInput').value);
    $('#customGameInput').onkeydown=e=>{if(e.key==='Enter')addCustomGame(e.target.value)};
    $('#pickAppBtn').onclick=pickInstalledApp;
    $('#customGamesList').onclick=e=>{const b=e.target.closest('[data-remove-game]');if(b)removeCustomGame(b.dataset.removeGame);};
    $('#selftestBtn').onclick=runSelftest;
    $('#diagBtn').onclick=async()=>{try{const path=(await jood(['diag'])).trim();openModal('诊断已导出',`<div>文件已保存到：</div><div style="margin-top:8px;color:var(--text);word-break:break-all">${esc(path)}</div>`,[{text:'完成',className:'primary'}]);}catch(e){toast(`导出失败：${e.message}`);}};
    $('#exportConfigBtn').onclick=async()=>{try{const path=(await jood(['config','export'])).trim();openModal('配置已导出',`<div>完整 config.json 已保存：</div><div style="margin-top:8px;color:var(--text);word-break:break-all">${esc(path)}</div><div style="margin-top:8px">同时更新 JoooScheduler_Config_latest.json，方便一键导入。</div>`,[{text:'完成',className:'primary'}]);}catch(e){toast(`导出配置失败：${e.message}`);}};
    $('#importConfigBtn').onclick=()=>openModal('导入配置','将读取 /sdcard/Download/ 中最新的 JoooScheduler_Config*.json。当前配置会先自动备份到 /data/adb/yumi_jooo/config_backups/。',[{text:'取消'},{text:'确认导入',className:'primary',onClick:async()=>{try{const path=(await jood(['config','import'])).trim();toast('配置导入成功');await refreshStatus(true);await loadGames();openModal('导入完成',`<div>已导入：</div><div style="margin-top:8px;color:var(--text);word-break:break-all">${esc(path)}</div>`,[{text:'完成',className:'primary'}]);}catch(e){toast(`导入失败：${e.message}`);}}}]);
    $('#reinstallSceneBtn').onclick=async()=>{try{await execAsync(`MODDIR=${shellQuote(state.root)} /system/bin/sh ${shellQuote(state.root+'/scripts/install_scene_backend.sh')}`);toast('Scene 后端已重装');await refreshStatus(true);}catch(e){toast(`重装失败：${e.message}`);}};
    $('#stopBtn').onclick=async()=>{try{await jood(['stop']);toast('游戏引擎已停止并恢复');await refreshStatus(true);}catch(e){toast(`停止失败：${e.message}`);}};
    $('#restoreBtn').onclick=()=>openModal('强制恢复原厂状态','将立即停止游戏控制器，并恢复进入游戏前保存的 CPU 上下限、governor 与线程亲和性。',[{text:'取消'},{text:'确认恢复',className:'danger',onClick:async()=>{try{await jood(['restore']);toast('原厂状态已恢复');await refreshStatus(true);}catch(e){toast(`恢复失败：${e.message}`);}}}]);
    $('#reloadLogsBtn').onclick=reloadLogs;
    document.addEventListener('visibilitychange',()=>{if(!document.hidden&&state.page==='overview')refreshStatus(true)});
  }

  async function init() {
    initModuleRoot(); bind();
    try { if (window.ksu && ksu.enableEdgeToEdge) ksu.enableEdgeToEdge(true); } catch (_) {}
    if (!hasKsu()) {
      $('#strategyTitle').textContent='请从 KernelSU 打开';
      $('#strategyDesc').textContent='此 WebUI 需要 KernelSU 提供的 ksu JavaScript 接口。';
      return;
    }
    await refreshStatus(true);
    state.timer=setInterval(()=>{if(!document.hidden&&state.page==='overview')refreshStatus(false)},2500);
  }

  init();
})();
