<script>
  import { onMount, onDestroy } from 'svelte'
  import { GetConfig, SaveConfig, TestHydracker, TestTMDB, TestOneFichier, TestSendCm, TestFTP, TestSeedbox, TestQBit, TestModSeedbox, TestNextcloud, TestUsenet, TestLihdl, HasSeedboxSettingsPassword, SetSeedboxSettingsPassword, VerifySeedboxSettingsPassword, ClearSeedboxSettingsPassword } from '../wailsjs/go/main/App.js'
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime.js'
  import HydrackerTab from './HydrackerTab.svelte'
  import { logEntries, addLog, clearLogs } from './logs.js'
  import logo from './assets/logo.png'
  import loginLogo from './assets/login-logo.png'
  import { ListCheckTorrents, ReseedFromLihdl, ReseedPrepare, ReseedExecute, SelectAnyTorrentFile, SelectMkvFile, GetVersion, StartWatchFolder, StopWatchFolder, IsWatching, CheckForUpdate, OpenBrowser, HistoryList, HistoryDelete, HistoryStats, DownloadUpdate, HasLihdlSettingsPassword, SetLihdlSettingsPassword, VerifyLihdlSettingsPassword, ClearLihdlSettingsPassword, IsLihdlPasswordManaged, IsHydrackerURLManaged, GetEffectiveHydrackerURL, FindHydrackerSources, FicheGetContent, FicheGetNfo, GetDDLFilename, GetUploaderStats, LoginUser, Logout, GetCurrentUser, TryAutoLogin, HashPassword, GetTeamConfig, BuildTeamJSON, FetchHydrackerAvatar, ChangeMyPassword, GetNzbFilenames, DeleteSeedboxByHash, MediaSearch, HydrackerSearch, TMDBGetByImdbID, TMDBGetProviders, HydrackerGetByID, HydrackerGetByTmdbID, HydrackerGetLienByID, HydrackerGetLienDetailByID, GetDDLFilenameByLienID, DownloadToDownloads, AutoReseedFromHydracker, AutoReseedDDLFromHydracker, AutoReseedFullFromTorrent, ListReseedRequests, ListMyLiens, ListMyTorrents, ListMyNzbs, DeleteMyLien, DeleteMyTorrent, DeleteMyNzb, DeleteTorrentAndFTP, ListSeedboxHashes, GetNexumIndex, GetSeedboxNexumIndex, DebugNexumMatch, TestNexum, TestSFTP, TestAllDebrid, TestIgdb, UpdateMyLien, UpdateMyTorrent, GetMetaQualities, ListTitlesSorted, GetUserProfile, ParseFilename, Notify } from '../wailsjs/go/main/App.js'

  // --- Tabs (réorganisés par workflow, 8 onglets principaux) ---
  const TABS = [
    { id: 'hydracker', label: '🚀 Publier' },
    // 'fiches', 'check', 'reseed' retirés — flow torrent + browsing supprimés
    { id: 'myuploads', label: '📤 Mes uploads' },
    { id: 'history',   label: '📚 Historique' },
    { id: 'logs',      label: '🔬 Logs' },           // fusion : Journal + API
    // 'manager' retiré : plus d'auth = plus de gestion utilisateurs
    { id: 'settings',  label: '⚙️ Réglages' },
  ]
  // Sous-onglets des tabs fusionnées
  let reseedSubTab = 'requests'   // 'requests' | 'url'
  let logsSubTab = 'journal'      // 'journal' | 'api'
  // Onglets visibles UNIQUEMENT pour certains pseudos (indépendant du rôle)
  const TABS_OWNER_ONLY = {}
  // Onglets réservés à certains rôles (override de la conf team.json)
  const TABS_ROLE_ONLY = {
    'manager': ['admin'],
  }
  let activeTab = 'hydracker'

  // --- Auth retirée (2026-07-21) : plus de login/password. L'identité vient
  // du token Hydracker (pour "Mes uploads" filter). Sans token → mode anonyme.
  let authState = 'ok'
  let loginPseudo = ''
  let loginPassword = ''
  let loginError = ''
  let loginLoading = false
  let myRole = ''
  let myUsername = ''
  let myAvatar = ''
  let myTitle = ''
  let myBadge = ''
  let myColor = ''
  let myTabs = []
  let myPermissions = []

  async function doLogin() {
    loginError = ''
    loginLoading = true
    try {
      const auth = await LoginUser(loginPseudo, loginPassword)
      applyAuth(auth)
      loginPassword = ''
      authState = 'ok'
      // Fetch avatar Hydracker en tâche de fond (silencieux si token absent/invalide)
      fetchAvatar()
    } catch (e) {
      loginError = String(e?.message || e).replace(/^Error:\s*/, '')
    } finally {
      loginLoading = false
    }
  }

  async function fetchAvatar() {
    try {
      const av = await FetchHydrackerAvatar()
      if (av) myAvatar = av
    } catch {}
  }

  function applyAuth(auth) {
    myUsername = auth?.username || ''
    myRole     = auth?.role     || ''
    myAvatar   = auth?.avatar   || ''
    myTitle    = auth?.title    || ''
    myBadge    = auth?.badge    || ''
    myColor    = auth?.color    || ''
    myTabs     = Array.isArray(auth?.tabs) ? auth.tabs : []
    myPermissions = Array.isArray(auth?.permissions) ? auth.permissions : []
    // Expose globalement pour que HydrackerTab puisse lire depuis Svelte context
    if (typeof window !== 'undefined') window.__myPermissions = myPermissions
  }

  async function doLogout() {
    try { await Logout() } catch {}
    myUsername = ''; myRole = ''; myAvatar = ''; myTitle = ''
    myBadge = ''; myColor = ''; myTabs = []; myPermissions = []
    if (typeof window !== 'undefined') window.__myPermissions = []
    loginPseudo = ''; loginPassword = ''; loginError = ''
    authState = 'login'
  }

  // --- Manager tab state ---
  let managerView = 'users'              // 'users' | 'roles'
  let managerLoading = false
  let managerError = ''
  let teamUsers = []                     // [{ pseudo, role, title }]
  let teamRoles = {}                     // { [key]: { badge, color, title, tabs: [] } }
  let teamOriginalJSON = ''              // pour détecter les changements
  let lastSavedJSON = ''                 // snapshot du dernier "Générer team.json" pour le state ✅ sauvegardé
  let managerNewPasswords = {}           // { pseudo: newPass } — hash côté Go au Générer

  let selectedRole = ''
  let addUserOpen = false
  let addRoleOpen = false
  let editUserOpen = false
  let editUserTarget = null              // { pseudo, role, title }
  let outputOpen = false
  let outputJSON = ''

  let newUserForm = { pseudo: '', password: '', role: 'user', title: '' }
  let newRoleForm = { slug: '', badge: '🟣', color: '#a78bfa', title: '' }

  const ROLE_COLORS = [
    { name: 'Doré',    hex: '#fbbf24' },
    { name: 'Argent',  hex: '#cbd5e1' },
    { name: 'Bronze',  hex: '#cd7f32' },
    { name: 'Bleu',    hex: '#60a5fa' },
    { name: 'Violet',  hex: '#a78bfa' },
    { name: 'Vert',    hex: '#7ef0c0' },
    { name: 'Rouge',   hex: '#ff6b6b' },
    { name: 'Orange',  hex: '#fb923c' },
    { name: 'Rose',    hex: '#f472b6' },
    { name: 'Cyan',    hex: '#22d3ee' },
  ]
  const ROLE_EMOJIS = ['🥇','🥈','🥉','🔵','🟣','🎬','💎','🤝','🏠','⭐','🔥','⚡','👑','🛡','🎭','🎨','🚀']

  // On référence teamRoles + teamUsers directement (pas via currentManagerSnapshot()),
  // sinon Svelte ne tracke pas les deps à travers l'appel de fonction et le bloc
  // ne re-run pas quand on toggle un tab/permission/user dans le Manager.
  $: managerDirty = managerOpen && JSON.stringify({ roles: teamRoles, users: teamUsers }) !== teamOriginalJSON
  // managerSaved = true tant que l'état actuel correspond exactement à ce qui a été
  // généré la dernière fois (= JSON copié dans le presse-papier). Auto-clear si l'user
  // continue à éditer après le "Générer team.json".
  $: managerSaved = lastSavedJSON !== '' && JSON.stringify({ roles: teamRoles, users: teamUsers }) === lastSavedJSON
  $: managerOpen = activeTab === 'manager' && myRole === 'admin'

  function currentManagerSnapshot() {
    return { roles: teamRoles, users: teamUsers }
  }

  async function loadManager() {
    managerError = ''
    managerLoading = true
    try {
      const cfg = await GetTeamConfig()
      teamRoles = cfg?.roles || {}
      teamUsers = Array.isArray(cfg?.users) ? cfg.users.map(u => ({...u})) : []
      managerNewPasswords = {}
      teamOriginalJSON = JSON.stringify(currentManagerSnapshot())
      // Sélectionne le premier rôle par défaut
      const keys = Object.keys(teamRoles)
      if (keys.length && !teamRoles[selectedRole]) selectedRole = keys[0]
    } catch (e) {
      managerError = String(e?.message || e).replace(/^Error:\s*/, '')
    } finally {
      managerLoading = false
    }
  }

  function manageToggleTab(roleKey, tabId) {
    const r = teamRoles[roleKey]
    if (!r) return
    const has = (r.tabs || []).includes(tabId)
    const nextTabs = has ? r.tabs.filter(x => x !== tabId) : [...(r.tabs || []), tabId]
    teamRoles = { ...teamRoles, [roleKey]: { ...r, tabs: nextTabs } }
  }
  function manageTogglePermission(roleKey, perm) {
    const r = teamRoles[roleKey]
    if (!r) return
    const cur = Array.isArray(r.permissions) ? r.permissions : []
    const has = cur.includes(perm)
    const nextPerms = has ? cur.filter(x => x !== perm) : [...cur, perm]
    teamRoles = { ...teamRoles, [roleKey]: { ...r, permissions: nextPerms } }
  }
  // Liste des permissions custom configurables dans Manager
  const ROLE_PERMISSIONS = [
    { id: 'torrent_admin', label: '👑 Bouton Torrent ADMIN', help: 'Autorise l\'utilisation du workflow Torrent ADMIN (NextCloud + qBittorrent ADMIN)' },
  ]

  function manageAddUser() {
    if (!newUserForm.pseudo || !newUserForm.password || !newUserForm.role) return
    const pseudo = newUserForm.pseudo.trim()
    // Anti-doublon (case-insensitive)
    if (teamUsers.some(u => u.pseudo.toLowerCase() === pseudo.toLowerCase())) {
      managerError = 'Ce pseudo existe déjà'
      return
    }
    teamUsers = [...teamUsers, { pseudo, role: newUserForm.role, title: newUserForm.title.trim() }]
    managerNewPasswords = { ...managerNewPasswords, [pseudo]: newUserForm.password }
    newUserForm = { pseudo: '', password: '', role: 'user', title: '' }
    addUserOpen = false
  }

  function manageEditUser() {
    if (!editUserTarget) return
    // Applique les modifs sur teamUsers
    teamUsers = teamUsers.map(u =>
      u.pseudo === editUserTarget.pseudo ? { ...u, role: editUserTarget.role, title: editUserTarget.title } : u
    )
    if (editUserTarget.newPassword) {
      managerNewPasswords = { ...managerNewPasswords, [editUserTarget.pseudo]: editUserTarget.newPassword }
    }
    editUserOpen = false
    editUserTarget = null
  }

  function manageDeleteUser(pseudo) {
    // Option C : pas te virer toi-même
    if (pseudo === myUsername) {
      managerError = 'Tu ne peux pas te supprimer toi-même'
      return
    }
    if (!confirm(`Supprimer l'utilisateur "${pseudo}" ?`)) return
    teamUsers = teamUsers.filter(u => u.pseudo !== pseudo)
    const { [pseudo]: _, ...rest } = managerNewPasswords
    managerNewPasswords = rest
  }

  function manageAddRole() {
    const slug = newRoleForm.slug.trim().toLowerCase().replace(/[^a-z0-9_-]/g, '')
    if (!slug) { managerError = 'Slug invalide'; return }
    if (teamRoles[slug]) { managerError = 'Ce rôle existe déjà'; return }
    teamRoles = {
      ...teamRoles,
      [slug]: {
        badge: newRoleForm.badge,
        color: newRoleForm.color,
        title: newRoleForm.title || slug,
        tabs: ['hydracker', 'settings'], // minimum par défaut
      },
    }
    selectedRole = slug
    newRoleForm = { slug: '', badge: '🟣', color: '#a78bfa', title: '' }
    addRoleOpen = false
  }

  function manageDeleteRole(roleKey) {
    // Option C : pas supprimer admin
    if (roleKey === 'admin') { managerError = 'Le rôle admin est protégé'; return }
    // Interdit si des users l'utilisent encore
    const stillUsed = teamUsers.filter(u => u.role === roleKey)
    if (stillUsed.length > 0) {
      managerError = `Impossible : ${stillUsed.length} utilisateur(s) ont ce rôle (${stillUsed.map(u=>u.pseudo).join(', ')})`
      return
    }
    if (!confirm(`Supprimer le rôle "${roleKey}" ?`)) return
    const { [roleKey]: _, ...rest } = teamRoles
    teamRoles = rest
    if (selectedRole === roleKey) selectedRole = Object.keys(teamRoles)[0] || ''
  }

  function manageUpdateRoleMeta(roleKey, field, value) {
    const r = teamRoles[roleKey]
    if (!r) return
    teamRoles = { ...teamRoles, [roleKey]: { ...r, [field]: value } }
  }

  async function manageGenerateAndCopy() {
    managerError = ''
    try {
      const json = await BuildTeamJSON(teamRoles, teamUsers, managerNewPasswords)
      outputJSON = json
      outputOpen = true
      try { await navigator.clipboard.writeText(json) } catch {}
      // Snapshot l'état actuel pour activer l'indicateur "Modifications sauvegardées"
      lastSavedJSON = JSON.stringify({ roles: teamRoles, users: teamUsers })
    } catch (e) {
      managerError = String(e?.message || e).replace(/^Error:\s*/, '')
    }
  }

  function manageCancel() {
    if (!confirm('Annuler toutes les modifs non sauvegardées ?')) return
    loadManager()
  }

  // Auto-load quand on ouvre l'onglet Manager
  $: if (activeTab === 'manager' && myRole === 'admin' && !managerLoading && teamOriginalJSON === '') {
    loadManager()
  }

  // --- Change my password (Réglages) ---
  let changePwdValue = ''
  let changePwdConfirm = ''
  let changePwdOutput = ''
  let changePwdError = ''
  let changePwdSuccess = false
  let changePwdLoading = false

  async function doChangePassword() {
    changePwdError = ''
    changePwdSuccess = false
    changePwdOutput = ''
    if (!changePwdValue || changePwdValue !== changePwdConfirm) {
      changePwdError = 'Les mots de passe ne correspondent pas'
      return
    }
    changePwdLoading = true
    try {
      const json = await ChangeMyPassword(changePwdValue)
      changePwdOutput = json
      try { await navigator.clipboard.writeText(json); changePwdSuccess = true } catch {}
      changePwdValue = ''
      changePwdConfirm = ''
    } catch (e) {
      changePwdError = String(e?.message || e).replace(/^Error:\s*/, '')
    } finally {
      changePwdLoading = false
    }
  }

  // Les permissions par rôle sont maintenant définies dans team.json
  // (section "roles") et l'app reçoit la liste `tabs` via LoginUser.
  // → Éditable via l'onglet 👥 Manager (admin).
  // Auth retirée : tous les onglets sont visibles (plus de rôle/permissions).
  $: visibleTabs = TABS

  // --- Config ---
  let cfg = {
    hydracker_token: '',
    tmdb_api_key: '',
    tmdb_proxy_url: 'https://tmdb.uklm.xyz',
    lihdl_base_url: '',
    one_fichier_api_key: '',
    sendcm_api_key: '',
    nexum_api_key: '',
    nexum_base_url: 'https://nexum-core.com',
    alldebrid_api_key: '',
    igdb_client_id: '',
    igdb_client_secret: '',
    elysium_api_token: '',
    usenet_host: '', usenet_port: 119, usenet_ssl: false,
    usenet_user: '', usenet_password: '', usenet_connections: 20,
    usenet_group: 'alt.binaries.test',
    parpar_redundancy: 5, parpar_threads: 8, parpar_slice_size: 768000,
    ftp_host: '', ftp_port: 21, ftp_user: '', ftp_password: '', ftp_path: '', ftp_use_sftp: false,
    private_ftp_host: '', private_ftp_port: 21, private_ftp_user: '', private_ftp_password: '', private_ftp_path: '',
    seedbox_url: '', seedbox_user: '', seedbox_password: '', seedbox_label: '',
    private_seedbox_url: '', private_seedbox_user: '', private_seedbox_password: '', private_seedbox_label: '',
    private_qbit_url: '', private_qbit_user: '', private_qbit_password: '',
    qbit_url: '', qbit_user: '', qbit_password: '',
    qbit_admin_url: '', qbit_admin_user: '', qbit_admin_password: '',
    nextcloud_admin_url: '', nextcloud_admin_user: '', nextcloud_admin_password: '', nextcloud_admin_path: '/',
    mod_seedbox_url: '', mod_seedbox_user: '', mod_seedbox_password: '',
    ftp_mod_host: '', ftp_mod_port: 21, ftp_mod_user: '', ftp_mod_password: '', ftp_mod_path: '/',
    tracker_url: '', torrent_piece_size: 8388608,
    lihdl_user: '', lihdl_password: '',
    watch_folder: '', watch_auto_start: false,
    proxy_url: '',
  }
  let appVersion = ''
  let watchRunning = false
  let updateInfo = null  // { available, current, latest, url }
  let cfgSaved = false
  let sidebarCollapsed = false
  let updateChecking = false
  let updateCheckMsg = ''

  // LiHDL : protection admin (index + recherche TMDB — bonus pour les admins
  // qui ont le mdp partagé, les autres s'en passent et utilisent l'API TMDB).
  let lihdlUnlocked = false
  let lihdlHasPassword = false
  let lihdlManaged = false // true = mdp imposé au build, user ne peut pas le modifier
  let hydrackerURLManaged = false // true = URL API imposée au build
  let lihdlModal = null  // null | 'unlock' | 'create' | 'change' | 'remove'
  let lihdlPwdInput = ''
  let lihdlPwdCurrent = ''
  let lihdlPwdNew = ''
  let lihdlPwdConfirm = ''
  let lihdlPwdError = ''

  async function checkLihdlPasswordStatus() {
    try { lihdlHasPassword = await HasLihdlSettingsPassword() } catch(e) { lihdlHasPassword = false }
    try { lihdlManaged = await IsLihdlPasswordManaged() } catch(e) { lihdlManaged = false }
  }

  // --- Sections SEEDBOX/FTP : plus de mdp, toujours ouvert ---
  let seedboxUnlocked = true
  let seedboxHasPassword = false
  let seedboxModal = null  // null | 'unlock' | 'create' | 'change' | 'remove'
  let seedboxPwdInput = ''
  let seedboxPwdCurrent = ''
  let seedboxPwdNew = ''
  let seedboxPwdConfirm = ''
  let seedboxPwdError = ''

  async function checkSeedboxPasswordStatus() {
    try { seedboxHasPassword = await HasSeedboxSettingsPassword() } catch(e) { seedboxHasPassword = false }
  }

  function openSeedboxLockModal() {
    seedboxPwdInput = ''; seedboxPwdCurrent = ''; seedboxPwdNew = ''; seedboxPwdConfirm = ''; seedboxPwdError = ''
    seedboxModal = seedboxHasPassword ? 'unlock' : 'create'
  }

  async function submitSeedboxPwd() {
    seedboxPwdError = ''
    try {
      if (seedboxModal === 'unlock') {
        const ok = await VerifySeedboxSettingsPassword(seedboxPwdInput)
        if (!ok) { seedboxPwdError = 'Mot de passe incorrect'; return }
        seedboxUnlocked = true
        seedboxModal = null
        // Notifie HydrackerTab que l'user vient d'être confirmé admin
        // → Torrent ADMIN devient visible dans la section Uploader via.
        window.dispatchEvent(new CustomEvent('admin-unlocked'))
      } else if (seedboxModal === 'create') {
        if (seedboxPwdNew.length < 4) { seedboxPwdError = 'Mot de passe trop court (4 min)'; return }
        if (seedboxPwdNew !== seedboxPwdConfirm) { seedboxPwdError = 'Les mots de passe ne correspondent pas'; return }
        await SetSeedboxSettingsPassword('', seedboxPwdNew)
        seedboxHasPassword = true
        seedboxUnlocked = true
        seedboxModal = null
      } else if (seedboxModal === 'change') {
        if (seedboxPwdNew.length < 4) { seedboxPwdError = 'Mot de passe trop court (4 min)'; return }
        if (seedboxPwdNew !== seedboxPwdConfirm) { seedboxPwdError = 'Les mots de passe ne correspondent pas'; return }
        await SetSeedboxSettingsPassword(seedboxPwdCurrent, seedboxPwdNew)
        seedboxModal = null
      } else if (seedboxModal === 'remove') {
        await ClearSeedboxSettingsPassword(seedboxPwdCurrent)
        seedboxHasPassword = false
        seedboxModal = null
      }
    } catch(e) { seedboxPwdError = String(e).replace('Error: ', '') }
  }

  function openLihdlLockModal() {
    lihdlPwdInput = ''; lihdlPwdCurrent = ''; lihdlPwdNew = ''; lihdlPwdConfirm = ''; lihdlPwdError = ''
    lihdlModal = lihdlHasPassword ? 'unlock' : 'create'
  }

  async function submitLihdlPwd() {
    lihdlPwdError = ''
    try {
      if (lihdlModal === 'unlock') {
        const ok = await VerifyLihdlSettingsPassword(lihdlPwdInput)
        if (!ok) { lihdlPwdError = 'Mot de passe incorrect'; return }
        lihdlUnlocked = true
        lihdlModal = null
      } else if (lihdlModal === 'create') {
        if (lihdlPwdNew.length < 4) { lihdlPwdError = 'Mot de passe trop court (4 min)'; return }
        if (lihdlPwdNew !== lihdlPwdConfirm) { lihdlPwdError = 'Les mots de passe ne correspondent pas'; return }
        await SetLihdlSettingsPassword('', lihdlPwdNew)
        lihdlHasPassword = true
        lihdlUnlocked = true
        lihdlModal = null
      } else if (lihdlModal === 'change') {
        if (lihdlPwdNew.length < 4) { lihdlPwdError = 'Mot de passe trop court (4 min)'; return }
        if (lihdlPwdNew !== lihdlPwdConfirm) { lihdlPwdError = 'Les mots de passe ne correspondent pas'; return }
        await SetLihdlSettingsPassword(lihdlPwdCurrent, lihdlPwdNew)
        lihdlModal = null
      } else if (lihdlModal === 'remove') {
        await ClearLihdlSettingsPassword(lihdlPwdCurrent)
        lihdlHasPassword = false
        lihdlModal = null
      }
    } catch(e) { lihdlPwdError = String(e).replace('Error: ', '') }
  }

  async function recheckUpdate() {
    updateChecking = true
    updateCheckMsg = ''
    addLog('UPDATE', '🔄 Vérification des mises à jour…')
    try {
      updateInfo = await CheckForUpdate()
      addLog('UPDATE', `GitHub: v${updateInfo?.latest || '?'} · Local: v${updateInfo?.current || '?'} · Dispo: ${updateInfo?.available}`)
      if (updateInfo?.available) {
        updateCheckMsg = `🆕 v${updateInfo.latest}`
      } else {
        updateCheckMsg = `✓ À jour (v${updateInfo?.current || ''})`
        setTimeout(() => { updateCheckMsg = '' }, 5000)
      }
    } catch(e) {
      updateCheckMsg = '✗ Erreur'
      addLog('UPDATE', `✗ Erreur check maj: ${e}`)
      setTimeout(() => { updateCheckMsg = '' }, 8000)
    }
    updateChecking = false
  }

  // Update modal
  let showUpdateModal = false
  let updateState = { stage: '', msg: '', percent: 0, downloading: false, downloadedPath: '' }

  async function startDownloadUpdate() {
    updateState = { stage: '', msg: '', percent: 0, downloading: true, downloadedPath: '' }
    try {
      const path = await DownloadUpdate()
      updateState = { ...updateState, downloading: false, downloadedPath: path, stage: 'done' }
    } catch(e) {
      updateState = { ...updateState, downloading: false, stage: 'error', msg: String(e) }
      addLog('UPDATE', '✗ ' + e)
    }
  }

  // Historique
  let histEntries = []
  let histFilter = ''      // '' | 'torrent' | 'nzb' | 'ddl'
  let histQuery = ''
  let histStats = { total: 0, ok: 0, error: 0, torrent: 0, nzb: 0, ddl: 0 }
  let histLoading = false

  async function loadHistory() {
    histLoading = true
    try {
      histEntries = await HistoryList(histFilter, histQuery, 0, 500) || []
      histStats = await HistoryStats() || histStats
    } catch(e) { addLog('HIST', '✗ ' + e) }
    histLoading = false
  }
  $: if (activeTab === 'history') loadHistory()

  async function deleteHistEntry(id) {
    if (!confirm('Supprimer cette entrée ?')) return
    try { await HistoryDelete(id); await loadHistory() } catch(e) {}
  }

  function formatDate(iso) {
    if (!iso) return ''
    const d = new Date(iso)
    return d.toLocaleString('fr-FR', { day:'2-digit', month:'2-digit', year:'2-digit', hour:'2-digit', minute:'2-digit' })
  }

  // NZB live status (partagé avec l'onglet NZB)
  let nzbStatus = ''
  let nzbParparPct = 0
  let nzbNyuuPct = 0
  let nzbNyuuSpeed = ''
  let nzbNyuuETA = ''
  let nzbNyuuArticles = ''
  let nzbDone = false
  let nzbResult = null  // { ok, message }

  // Show passwords
  let showPwd = {}

  // Reseed
  let reseedTorrentPath = ''
  let reseedMkvPath = ''
  let reseedPrepLoading = false
  let reseedPrep = null  // { torrent_name, first_file_name, size, info_hash, search, hydracker_fiche }
  let reseedRunning = false
  let reseedStage = ''
  let reseedMsg = ''
  let reseedPct = 0
  let reseedSpeed = 0

  async function reseedPickTorrent() {
    try {
      const p = await SelectAnyTorrentFile()
      if (p) { reseedTorrentPath = p; reseedPrep = null }
    } catch(e) { addLog('RES', '✗ ' + e) }
  }
  async function reseedPickMkv() {
    try {
      const p = await SelectMkvFile()
      if (p) reseedMkvPath = p
    } catch(e) { addLog('RES', '✗ ' + e) }
  }
  async function reseedAnalyze() {
    if (!reseedTorrentPath) return
    reseedPrepLoading = true
    reseedPrep = null
    try { reseedPrep = await ReseedPrepare(reseedTorrentPath) }
    catch(e) { addLog('RES', '✗ analyse: ' + e) }
    reseedPrepLoading = false
  }
  async function reseedConfirm() {
    if (!reseedTorrentPath || !reseedMkvPath) return
    reseedRunning = true
    reseedStage = 'start'; reseedMsg = 'Démarrage…'; reseedPct = 0; reseedSpeed = 0
    try { await ReseedExecute(reseedTorrentPath, reseedMkvPath) }
    catch(e) { addLog('RES', '✗ ' + e) }
    reseedRunning = false
  }
  function reseedReset() {
    reseedTorrentPath = ''; reseedMkvPath = ''; reseedPrep = null
    reseedStage = ''; reseedMsg = ''; reseedPct = 0; reseedSpeed = 0
  }

  // Télécharge un .torrent / NZB directement dans ~/Downloads (via signed URL Hydracker)
  async function downloadFromHydracker(url, name) {
    if (!url) return
    try {
      const path = await DownloadToDownloads(url, name)
      addLog('RES', `⬇ Téléchargé : ${path}`)
      try { Notify('✓ Téléchargement terminé', name) } catch(e) {}
    } catch(e) { addLog('RES', '✗ ' + e) }
  }

  // --- Auto-reseed : workflow standalone (sans .torrent local, juste un ID Hydracker) ---
  let autoReseedInput = ''            // URL Hydracker ou ID brut
  let autoReseedSaison = 0
  let autoReseedEpisode = 0
  let autoReseedLoading = false
  let autoReseedStatus = ''           // dernier message du workflow
  let autoReseedResult = null         // { torrent_id, torrent_name, size_bytes, seedbox_path }
  let autoReseedError = ''

  function parseHydrackerID(input) {
    const s = (input || '').trim()
    if (!s) return 0
    // URL avec /titles/NNN ou /title/NNN
    const m = s.match(/\/titles?\/(\d+)/)
    if (m) return parseInt(m[1])
    // Juste un nombre
    if (/^\d+$/.test(s)) return parseInt(s)
    return 0
  }

  async function launchAutoReseed() {
    autoReseedError = ''
    autoReseedResult = null
    const id = parseHydrackerID(autoReseedInput)
    if (!id) { autoReseedError = 'ID Hydracker invalide — colle une URL /titles/XXX ou un nombre'; return }
    if (!cfg.seedbox_url) { autoReseedError = 'Seedbox non configurée (Réglages)'; return }
    autoReseedLoading = true
    autoReseedStatus = 'Démarrage…'
    addLog('AUTO', `▶ Auto-reseed torrent fiche #${id} saison=${autoReseedSaison} épisode=${autoReseedEpisode}`)
    try {
      const r = await AutoReseedFromHydracker(id, autoReseedSaison || 0, autoReseedEpisode || 0, 0, 0)
      autoReseedResult = r
      addLog('AUTO', `✓ Torrent #${r.torrent_id} ${r.torrent_name} → seedbox OK`)
      try { Notify('✓ Auto-reseed terminé', r.torrent_name) } catch(e) {}
    } catch(e) {
      autoReseedError = String(e?.message || e)
      addLog('AUTO', `✗ ${autoReseedError}`)
    }
    autoReseedLoading = false
  }

  // Variante DDL → FTP : quand la fiche n'a pas de torrent partagé via API mais
  // qu'un DDL 1fichier existe, on télécharge depuis le DDL et on pousse direct
  // sur le FTP configuré (streaming, pas de passage sur le disque local).
  let autoReseedDDLProgress = { percent: 0, speed_mb: 0, bytes: 0, total: 0 }
  async function launchAutoReseedDDL() {
    autoReseedError = ''
    autoReseedResult = null
    autoReseedDDLProgress = { percent: 0, speed_mb: 0, bytes: 0, total: 0 }
    const id = parseHydrackerID(autoReseedInput)
    if (!id) { autoReseedError = 'ID Hydracker invalide — colle une URL /titles/XXX ou un nombre'; return }
    if (!cfg.ftp_host) { autoReseedError = 'FTP non configuré (Réglages)'; return }
    if (!cfg.one_fichier_api_key) { autoReseedError = 'Clé API 1fichier non configurée (Réglages) — requise pour le DDL'; return }
    autoReseedLoading = true
    autoReseedStatus = 'Démarrage DDL…'
    addLog('AUTO', `▶ Auto-reseed DDL fiche #${id} saison=${autoReseedSaison} épisode=${autoReseedEpisode}`)
    try {
      const r = await AutoReseedDDLFromHydracker(id, autoReseedSaison || 0, autoReseedEpisode || 0, 0, 0)
      autoReseedResult = { torrent_id: r.lien_id, torrent_name: r.filename, size_bytes: r.size_bytes, seedbox_path: 'FTP : ' + r.ftp_remote_name + ' (host: ' + r.host + ')' }
      addLog('AUTO', `✓ DDL #${r.lien_id} ${r.filename} → FTP OK`)
      try { Notify('✓ Auto-reseed DDL terminé', r.filename) } catch(e) {}
    } catch(e) {
      autoReseedError = String(e?.message || e)
      addLog('AUTO', `✗ ${autoReseedError}`)
    }
    autoReseedLoading = false
  }

  // Ouvre l'onglet Hydracker avec le .torrent + la fiche Hydracker pré-remplis.
  // Utile quand on a un .torrent existant (sans seedbox) : on saute la phase de
  // création/FTP et on poste directement le fichier à Hydracker.
  function reseedOpenInHydracker(prep) {
    if (!prep) return
    activeTab = 'hydracker'
    window.dispatchEvent(new CustomEvent('hydracker:preload-torrent', { detail: {
      torrentPath: reseedTorrentPath,
      mkvPath: reseedMkvPath || '',
      hydrackerFiche: prep.hydracker_fiche,
      tmdbSearch: prep.search,
      torrentName: prep.torrent_name,
    } }))
  }

// --- Fiches : recherche + drill-down sur toutes les fiches du site ---
  let fichesMode = 'name'        // 'name' | 'hydracker_id' | 'tmdb_id'
  let fichesQuery = ''
  let fichesResults = []          // []PartialTitle
  let fichesLoading = false
  let fichesError = ''
  let fichesSelected = null       // fiche actuellement ouverte en détail
  let fichesContent = null        // {liens, nzbs, torrents} de la fiche sélectionnée
  let fichesContentLoading = false
  let fichesContentTab = 'torrents'  // 'torrents' | 'nzbs' | 'liens'
  let nzbFilenames = {}           // { [nzbId]: { state, files[], error } }

  // groupBySeason : transforme [{saison, episode, ...}] en [{ season:int, items:[], hasFullSeason:bool }]
  // Trié : saisons décroissantes, puis full_saison en haut, puis épisodes dans l'ordre.
  function groupBySeason(items) {
    const bySeason = new Map()
    for (const it of items || []) {
      const s = parseInt(it.saison || it.season || 0) || 0
      if (!bySeason.has(s)) bySeason.set(s, [])
      bySeason.get(s).push(it)
    }
    const seasons = [...bySeason.entries()]
      .map(([season, items]) => ({
        season,
        items: [...items].sort((a, b) => {
          // full_saison d'abord, puis par épisode croissant
          if (a.full_saison && !b.full_saison) return -1
          if (!a.full_saison && b.full_saison) return 1
          return (parseInt(a.episode||0)||0) - (parseInt(b.episode||0)||0)
        }),
      }))
      .sort((a, b) => b.season - a.season) // saisons décroissantes (dernière en haut)
    return seasons
  }

  async function loadNzbFilenames(nzbs) {
    for (const n of nzbs || []) {
      if (nzbFilenames[n.id]) continue
      nzbFilenames = { ...nzbFilenames, [n.id]: { state: 'loading' } }
      try {
        const files = await GetNzbFilenames(n.id)
        nzbFilenames = { ...nzbFilenames, [n.id]: { state: 'ok', files: files || [] } }
      } catch (e) {
        nzbFilenames = { ...nzbFilenames, [n.id]: { state: 'err', error: String(e?.message || e) } }
      }
    }
  }

  $: if (fichesContentTab === 'nzbs' && fichesContent?.nzbs?.nzbs?.length) {
    loadNzbFilenames(fichesContent.nzbs.nzbs)
  }

  async function fichesSearch() {
    fichesError = ''
    fichesResults = []
    fichesSelected = null
    const q = (fichesQuery || '').trim()
    if (!q) return
    fichesLoading = true
    try {
      if (fichesMode === 'name') {
        fichesResults = await HydrackerSearch(q) || []
      } else if (fichesMode === 'hydracker_id') {
        const id = parseInt(q.match(/\d+/)?.[0] || '0')
        if (!id) { fichesError = 'ID Hydracker invalide'; fichesLoading = false; return }
        const t = await HydrackerGetByID(id)
        fichesResults = t ? [t] : []
      } else if (fichesMode === 'tmdb_id') {
        const id = parseInt(q.match(/\d+/)?.[0] || '0')
        if (!id) { fichesError = 'ID TMDB invalide'; fichesLoading = false; return }
        const t = await HydrackerGetByTmdbID(id)
        fichesResults = t ? [t] : []
      }
      if (!fichesResults.length) fichesError = 'Aucune fiche trouvée'
    } catch(e) {
      fichesError = String(e?.message || e)
    }
    fichesLoading = false
  }

  // État bonus pour la fiche détail (note IMDb + watch providers via proxytmdb)
  let fichesProviders = null      // map[country]CountryProviders ou null
  let fichesProvidersLoading = false
  let fichesImdbInfo = null       // tmdb.Movie avec note_imdb

  async function fichesOpen(fiche) {
    fichesSelected = fiche
    fichesContent = null
    fichesContentLoading = true
    fichesContentTab = 'torrents'
    fichesProviders = null
    fichesImdbInfo = null
    try {
      fichesContent = await FicheGetContent(fiche.id)
    } catch(e) {
      addLog('FICHE', '✗ ' + e)
    }
    fichesContentLoading = false

    // Bonus async (non-bloquant) : fetch watch providers + note IMDb
    if (fiche.tmdb_id) {
      const mediaType = fiche.type === 'series' ? 'tv' : 'movie'
      fichesProvidersLoading = true
      TMDBGetProviders(fiche.tmdb_id, mediaType)
        .then(p => { fichesProviders = p || {} })
        .catch(() => {})
        .finally(() => { fichesProvidersLoading = false })
    }
    if (fiche.imdb_id) {
      TMDBGetByImdbID(fiche.imdb_id)
        .then(m => { fichesImdbInfo = m })
        .catch(() => {})
    }
  }

  function fichesBackToResults() {
    fichesSelected = null
    fichesContent = null
    fichesProviders = null
    fichesImdbInfo = null
  }

  async function downloadLienFromFiche(lien) {
    try {
      addLog('FICHE', `⬇ Téléchargement lien #${lien.id}…`)
      await DownloadToDownloads(lien.download_url || lien.lien || '', (lien.name || 'hydracker-lien-' + lien.id))
      addLog('FICHE', `✓ Lien #${lien.id} téléchargé dans Downloads`)
    } catch(e) { addLog('FICHE', '✗ ' + e) }
  }

  // --- Modale NFO ---
  let nfoModal = null  // { kind, id, title, html, loading, error }
  async function openNfo(kind, id, title) {
    nfoModal = { kind, id, title, html: '', loading: true, error: '' }
    try {
      const html = await FicheGetNfo(kind, id)
      if (!html || !html.trim()) {
        nfoModal = { ...nfoModal, loading: false, error: 'Aucun NFO disponible pour cet item.' }
      } else {
        nfoModal = { ...nfoModal, loading: false, html }
      }
    } catch(e) {
      nfoModal = { ...nfoModal, loading: false, error: String(e) }
    }
  }
  function closeNfo() { nfoModal = null }

  // --- DDL filename resolver (cache mémoire pendant la session) ---
  // ddlFilenames[lien.id] = { state: 'loading'|'ok'|'err', filename, error }
  // Queue séquentielle keyée par lien ID (l'URL débridée change à chaque debrid).
  let ddlFilenames = {}
  let ddlQueue = []  // [lienId]
  let ddlQueueRunning = false
  async function processDDLQueue() {
    if (ddlQueueRunning) return
    ddlQueueRunning = true
    addLog('FICHE', `processDDLQueue start (${ddlQueue.length} en attente)`)
    while (ddlQueue.length > 0) {
      const lienId = ddlQueue.shift()
      addLog('FICHE', `→ debrid lien #${lienId}`)
      let attempt = 0
      while (attempt < 2) {
        try {
          const r = await GetDDLFilenameByLienID(lienId)
          addLog('FICHE', `← lien #${lienId} : url=${r?.url || '∅'} filename=${r?.filename || '∅'} (keys: ${Object.keys(r || {}).join(',')})`)
          ddlFilenames = { ...ddlFilenames, [lienId]: { state: 'ok', filename: r?.filename || '', url: r?.url || '' } }
          break
        } catch(e) {
          const msg = String(e)
          addLog('FICHE', `✗ lien #${lienId} : ${msg}`)
          if (attempt === 0 && /429|too many/i.test(msg)) {
            attempt++
            await new Promise(r => setTimeout(r, 2500))
            continue
          }
          ddlFilenames = { ...ddlFilenames, [lienId]: { state: 'err', error: msg } }
          break
        }
      }
      await new Promise(r => setTimeout(r, 1500))
    }
    ddlQueueRunning = false
    addLog('FICHE', `processDDLQueue done`)
  }
  function resolveDDLName(lienId) {
    if (!lienId) return
    if (ddlFilenames[lienId]) return  // déjà résolu/en cours/échoué
    if (ddlQueue.includes(lienId)) return  // déjà en queue
    ddlFilenames = { ...ddlFilenames, [lienId]: { state: 'loading' } }
    ddlQueue.push(lienId)
    processDDLQueue()
  }
  // Auto-résolution batch dès qu'on bascule sur l'onglet "Liens DDL" d'une fiche.
  // On passe juste l'ID stable au backend qui se débrouille pour debrid + HEAD.
  // L'URL est volatile (chaque debrid en regénère une nouvelle) donc inutile
  // de la stocker côté frontend.
  $: if (fichesContentTab === 'liens' && fichesContent?.liens?.liens?.length) {
    for (const l of fichesContent.liens.liens) {
      resolveDDLName(l.id)
    }
  }

  // Formatters partagés pour les cartes Fiches
  function fmtSize(b) {
    if (!b) return ''
    if (b >= 1e9) return (b/1e9).toFixed(2) + ' GB'
    if (b >= 1e6) return (b/1e6).toFixed(0) + ' MB'
    return (b/1e3).toFixed(0) + ' KB'
  }
  // Drapeaux pour les langues les plus courantes
  const LANG_FLAGS = {
    'TrueFrench': '🇫🇷', 'French': '🇫🇷', 'French (Canada)': '🇨🇦', 'FRENCH AD': '🇫🇷👁',
    'English': '🇬🇧', 'Spanish': '🇪🇸', 'German': '🇩🇪', 'Italian': '🇮🇹',
    'Japanese': '🇯🇵', 'Korean': '🇰🇷', 'Chinese': '🇨🇳', 'Portuguese': '🇵🇹',
    'Dutch': '🇳🇱', 'Russian': '🇷🇺', 'Arabic': '🇸🇦', 'Hindi': '🇮🇳',
  }
  function langFlag(name) { return LANG_FLAGS[name] || '🌐' }

// --- Check Torrent : nouvelle vue "Mes seeds Hydracker" ---
  let mySeedsTorrents = []          // []TorrentItem depuis /admin/torrents?author=Gandalf
  let mySeedsLoading = false
  let mySeedsError = ''
  let mySeedsPage = 1
  let mySeedsTotalPages = 1
  let mySeedsFilter = 'all'         // 'all' | '0seed' | 'low' | 'ok'
  let mySeedsActioning = {}         // { [torrentId]: true } pendant auto-reseed
  let checkActionProg = {}          // { [torrentId]: {stage, percent, speed_mb, msg} }
  let checkActionActiveID = 0       // ID du torrent en cours d'action (pour router les events)

  // Bouton "Parcourir local" : check si un MKV local correspond à une fiche Hydracker
  let localMkvCheck = null          // { filename, parsed, hydrackerFiche, content, error }
  let localMkvChecking = false

  // Set des hashes que l'user a actuellement sur sa seedbox (lowercase).
  // Rempli au load via ListSeedboxHashes — sert à filtrer Check Torrent pour
  // ne montrer que les torrents qu'on a réellement encore en seed.
  let seedboxHashes = new Set()
  let seedboxHashesLoaded = false
  let onlyMine = true   // toggle "présents seulement" / "tous les Hydracker"
  // Vue dédiée 3+ seeds (scan toutes les pages)
  let topSeedsView = false           // si true, affiche la vue dédiée 3+ seeds
  let topSeedsTorrents = []          // torrents avec >= 3 seeds, agrégés sur toutes les pages
  let topSeedsScanning = false
  let topSeedsScanProg = ''
  let topSeedsSelected = new Set()
  let topSeedsBulkDeleting = false
  let topSeedsBulkResult = ''

  // Index Nexum : map lowercase info_hash → {id, name, seeders, created_at, …}
  let nexumIndex = {}
  let nexumLoaded = false

  async function loadMySeeds() {
    mySeedsLoading = true
    mySeedsError = ''
    try {
      const r = await ListMyTorrents(myUsername, mySeedsPage)
      mySeedsTorrents = r?.pagination?.data || []
      mySeedsTotalPages = r?.pagination?.last_page || 1
      // Tri local par seeders ASC pour mettre les "à risque" en haut
      mySeedsTorrents.sort((a, b) => (a.seeders || 0) - (b.seeders || 0))
    } catch(e) {
      mySeedsError = String(e?.message || e)
      addLog('SEED', '✗ ' + mySeedsError)
    }
    mySeedsLoading = false
    loadSeedboxHashes()
  }

  async function scanAllPagesFor3PlusSeeds() {
    topSeedsScanning = true
    topSeedsTorrents = []
    topSeedsScanProg = 'Scan en cours…'
    try {
      // Page 1 d'abord pour connaître le nombre total
      let r = await ListMyTorrents(myUsername, 1)
      const lastPage = r?.pagination?.last_page || 1
      let acc = []
      const pushIfTopSeed = arr => {
        for (const t of (arr || [])) {
          if ((t.seeders || 0) >= 3) acc.push(t)
        }
      }
      pushIfTopSeed(r?.pagination?.data)
      topSeedsScanProg = `Page 1/${lastPage} · ${acc.length} torrents 3+ seeds`
      for (let p = 2; p <= lastPage; p++) {
        const rp = await ListMyTorrents(myUsername, p)
        pushIfTopSeed(rp?.pagination?.data)
        topSeedsScanProg = `Page ${p}/${lastPage} · ${acc.length} torrents 3+ seeds`
      }
      // Tri seeds DESC
      acc.sort((a, b) => (b.seeders || 0) - (a.seeders || 0))
      topSeedsTorrents = acc
      topSeedsScanProg = `✓ Scan terminé : ${acc.length} torrents avec 3+ seeds sur ${lastPage} page${lastPage > 1 ? 's' : ''}`
      addLog('SEED', topSeedsScanProg)
    } catch (e) {
      topSeedsScanProg = '✗ ' + String(e?.message || e)
      addLog('SEED', '✗ scan 3+ : ' + e)
    }
    topSeedsScanning = false
  }

  function toggleTopSeedSelect(id) {
    if (topSeedsSelected.has(id)) topSeedsSelected.delete(id)
    else topSeedsSelected.add(id)
    topSeedsSelected = new Set(topSeedsSelected)
  }
  function toggleTopSeedSelectAll() {
    if (topSeedsSelected.size === topSeedsTorrents.length) {
      topSeedsSelected = new Set()
    } else {
      topSeedsSelected = new Set(topSeedsTorrents.map(t => t.id))
    }
  }
  $: topSeedsSelectedSize = topSeedsTorrents
    .filter(t => topSeedsSelected.has(t.id))
    .reduce((acc, t) => acc + (t.taille || t.size || 0), 0)

  async function bulkDeleteTopSeeds() {
    const ids = [...topSeedsSelected]
    if (!ids.length) return
    if (!confirm(`Supprimer ${ids.length} torrent(s) sur Hydracker + leur fichier sur ton FTP ?\n⚠ Action irréversible.`)) return
    topSeedsBulkDeleting = true
    topSeedsBulkResult = ''
    let ok = 0, errs = []
    for (const id of ids) {
      try {
        await DeleteTorrentAndFTP(id)
        ok++
        topSeedsTorrents = topSeedsTorrents.filter(t => t.id !== id)
        topSeedsSelected.delete(id)
      } catch (e) {
        errs.push('#' + id + ': ' + String(e?.message || e).replace(/^Error:\s*/, ''))
      }
    }
    topSeedsSelected = new Set(topSeedsSelected)
    topSeedsBulkDeleting = false
    topSeedsBulkResult = `✅ ${ok}/${ids.length} supprimé(s)${errs.length ? ' · ❌ ' + errs.length + ' erreur(s)' : ''}`
    addLog('SEED', `Bulk delete 3+ : ${ok}/${ids.length} OK${errs.length ? ' · errs: ' + errs.join(' | ') : ''}`)
  }

  async function loadSeedboxHashes() {
    try {
      const list = await ListSeedboxHashes()
      seedboxHashes = new Set((list || []).map(h => h.toLowerCase()))
      seedboxHashesLoaded = true
      addLog('SEED', `📡 Seedbox : ${seedboxHashes.size} torrents trouvés sur ta seedbox`)
    } catch(e) {
      seedboxHashesLoaded = false
      addLog('SEED', `⚠ Liste seedbox indispo : ${e}`)
    }
  }

  // normalizeName : strip . _ - espaces, lower → match robuste Hydracker ↔ Nexum
  // (les 2 trackers recalculent le hash différemment car ils remplacent l'announce)
  function normalizeNameJS(s) {
    if (!s) return ''
    return String(s).toLowerCase().replace(/[^a-z0-9]/g, '')
  }

  let nexumLoading = false
  let nexumError = ''

  async function loadNexumIndex(silent = false) {
    if (!cfg.nexum_api_key) { nexumLoaded = false; nexumIndex = {}; return }
    nexumLoading = true
    nexumError = ''
    try {
      // Approche seedbox-first : on scanne UNIQUEMENT les torrents que t'as sur ta seedbox
      // (les .torrent Nexum y sont aussi puisque même seedbox), pour chacun on demande
      // à Nexum les stats par info_hash. Beaucoup + rapide qu'un pull catalogue Nexum entier.
      const idx = await GetSeedboxNexumIndex()
      nexumIndex = idx || {}
      nexumLoaded = true
      const count = Object.keys(nexumIndex).filter(k => k.startsWith('h:')).length
      if (!silent) addLog('SEED', `📡 Nexum : ${count} torrents matchés via seedbox`)
    } catch(e) {
      nexumLoaded = false
      nexumError = String(e?.message || e)
      if (!silent) addLog('SEED', `⚠ Nexum index indispo : ${nexumError}`)
    }
    nexumLoading = false
  }

  async function debugNexumFirst() {
    if (!nexumLoaded || !mySeedsTorrents.length) return
    const t = mySeedsTorrents[0]
    const name = t.torrent_name || t.name || ''
    const hash = t.info_hash || t.hash || ''
    addLog('SEED', `🔬 Debug t[0]: name="${name}" hash="${hash}"`)
    try {
      const d = await DebugNexumMatch(nexumIndex, name, hash)
      addLog('SEED', `🔬 hashLower=${d.hyd_hash_lower} nameNorm=${d.hyd_name_norm}`)
      addLog('SEED', `🔬 hashKeyExists=${d.hash_key_exists} nameKeyExists=${d.name_key_exists}`)
      addLog('SEED', `🔬 index: ${d.index_hash_count} hash keys, ${d.index_name_count} name keys`)
      addLog('SEED', `🔬 sample names: ${(d.sample_name_keys||[]).join(' | ')}`)
    } catch(e) {
      addLog('SEED', `🔬 debug err: ${e}`)
    }
  }

  // nexumLookup : tente match par info_hash, fallback nom normalisé.
  // Retourne {seeders, leechers, id, name} ou null.
  function nexumLookup(t) {
    if (!nexumLoaded) return null
    const h = (t.info_hash || t.hash || '').toLowerCase()
    if (h) {
      const m = nexumIndex['h:' + h]
      if (m) return m
    }
    const name = t.torrent_name || t.name || ''
    if (name) {
      const m = nexumIndex['n:' + normalizeNameJS(name)]
      if (m) return m
    }
    return null
  }

  $: filteredMySeeds = mySeedsTorrents.filter(t => {
    // Filtre 1 : présent sur ta seedbox (si toggle activé et liste chargée)
    if (onlyMine && seedboxHashesLoaded) {
      const h = (t.info_hash || t.hash || '').toLowerCase()
      if (!h || !seedboxHashes.has(h)) return false
    }
    // Filtre 2 : seeders count
    const s = t.seeders || 0
    if (mySeedsFilter === '0seed') return s === 0
    if (mySeedsFilter === 'low') return s > 0 && s <= 2
    if (mySeedsFilter === 'ok') return s >= 3
    return true
  })

  $: if (activeTab === 'check') loadMySeeds()

  async function autoReseedFromCheck(t) {
    if (!t?.title_id) return
    mySeedsActioning = { ...mySeedsActioning, [t.id]: true }
    checkActionActiveID = t.id
    checkActionProg = { ...checkActionProg, [t.id]: { stage: 'start', percent: 0, speed_mb: 0, msg: 'Upload .torrent…' } }
    addLog('SEED', `▶ Torrent → seedbox #${t.id} (fiche #${t.title_id})`)
    try {
      const r = await AutoReseedFromHydracker(t.title_id, t.saison || 0, t.episode || 0, 0, 0)
      checkActionProg = { ...checkActionProg, [t.id]: { stage: 'done', percent: 100, speed_mb: 0, msg: 'Terminé — recheck OK' } }
      addLog('SEED', `✓ Reseed OK → ${r.torrent_name}`)
      try { Notify('✓ Reseed OK', t.torrent_name || t.name) } catch(e) {}
      setTimeout(loadMySeeds, 1500)
    } catch(e) {
      checkActionProg = { ...checkActionProg, [t.id]: { stage: 'error', percent: 0, speed_mb: 0, msg: String(e) } }
      addLog('SEED', `✗ Reseed #${t.id} : ${e}`)
    }
    mySeedsActioning = { ...mySeedsActioning, [t.id]: false }
    checkActionActiveID = 0
    setTimeout(() => { const { [t.id]: _, ...rest } = checkActionProg; checkActionProg = rest }, 4000)
  }

  // Reseed complet : DDL 1fichier → FTP (nom exact torrent) + .torrent seedbox + recheck
  async function fullReseedFromCheck(t) {
    if (!t?.title_id || !t?.id) return
    mySeedsActioning = { ...mySeedsActioning, [t.id]: true }
    checkActionActiveID = t.id
    checkActionProg = { ...checkActionProg, [t.id]: { stage: 'start', percent: 0, speed_mb: 0, msg: 'DDL 1fichier…' } }
    addLog('SEED', `▶ Reseed complet torrent #${t.id} (DDL→FTP+seedbox+recheck)`)
    try {
      const r = await AutoReseedFullFromTorrent(t.id, t.title_id, t.saison || 0, t.episode || 0)
      checkActionProg = { ...checkActionProg, [t.id]: { stage: 'done', percent: 100, speed_mb: 0, msg: `Terminé — ${r.rechecked ? 'recheck OK' : 'sans recheck'}` } }
      addLog('SEED', `✓ ${r.expected_filename} → FTP OK · seedbox ${r.seedbox_path}${r.rechecked ? ' · recheck OK' : ''}`)
      try { Notify('✓ Reseed complet OK', r.expected_filename) } catch(e) {}
      setTimeout(loadMySeeds, 1500)
    } catch(e) {
      checkActionProg = { ...checkActionProg, [t.id]: { stage: 'error', percent: 0, speed_mb: 0, msg: String(e) } }
      addLog('SEED', `✗ Reseed complet #${t.id} : ${e}`)
    }
    mySeedsActioning = { ...mySeedsActioning, [t.id]: false }
    checkActionActiveID = 0
    setTimeout(() => { const { [t.id]: _, ...rest } = checkActionProg; checkActionProg = rest }, 4000)
  }

  // --- Suppression complète d'un torrent (Hydracker + FTP) ---
  let deleteTorrentModal = null   // null | { torrent, loading, result, error }
  function askDeleteTorrent(t) {
    deleteTorrentModal = { torrent: t, loading: false, result: null, error: '' }
  }
  async function confirmDeleteTorrent() {
    if (!deleteTorrentModal?.torrent) return
    const t = deleteTorrentModal.torrent
    deleteTorrentModal = { ...deleteTorrentModal, loading: true, error: '' }
    mySeedsActioning = { ...mySeedsActioning, [t.id]: true }
    addLog('DEL', `▶ Suppression torrent #${t.id} + fichier(s) FTP`)
    try {
      const r = await DeleteTorrentAndFTP(t.id)
      deleteTorrentModal = { ...deleteTorrentModal, loading: false, result: r }
      addLog('DEL', `✓ FTP ${r.used_ftp || 'rien trouvé'} · ${r.ftp_deleted?.length || 0} fichier(s) supprimé(s) · seedbox ${r.seedbox_ok ? 'OK' : 'KO'}${r.siblings_erased?.length ? ' · ' + r.siblings_erased.length + ' sibling(s) effacé(s)' : ''}`)
      try { Notify('🗑 Torrent supprimé', t.torrent_name || t.name || ('#' + t.id)) } catch(e) {}
      // Retire l'item de toutes les listes possibles (My Seeds, Check Torrent, Top Seeds)
      mySeedsTorrents = mySeedsTorrents.filter(x => x.id !== t.id)
      checkTorrents = checkTorrents.filter(x => x.id !== t.id && x.hash !== t.hash)
      topSeedsTorrents = topSeedsTorrents.filter(x => x.id !== t.id)
      setTimeout(loadMySeeds, 1500)
    } catch(e) {
      deleteTorrentModal = { ...deleteTorrentModal, loading: false, error: String(e?.message || e) }
      addLog('DEL', `✗ Suppr #${t.id} : ${e}`)
    }
    mySeedsActioning = { ...mySeedsActioning, [t.id]: false }
  }
  function closeDeleteTorrentModal() { deleteTorrentModal = null }

  async function checkLocalMkv() {
    try {
      const path = await SelectMkvFile()
      if (!path) return
      const filename = path.split(/[\\/]/).pop()
      localMkvChecking = true
      localMkvCheck = { filename, parsed: null, hydrackerFiche: null, content: null, error: '' }

      // 1. Parser le nom
      const parsed = await ParseFilename(filename)
      localMkvCheck.parsed = parsed

      // 2. Cherche la fiche Hydracker par titre
      if (parsed?.title) {
        const results = await HydrackerSearch(parsed.title)
        if (results?.length > 0) {
          localMkvCheck.hydrackerFiche = results[0]
          // 3. Récupère le contenu de la fiche (torrents + nzbs + liens)
          localMkvCheck.content = await FicheGetContent(results[0].id)
        } else {
          localMkvCheck.error = 'Aucune fiche Hydracker trouvée pour "' + parsed.title + '"'
        }
      } else {
        localMkvCheck.error = 'Pas de titre extrait du nom de fichier'
      }
    } catch(e) {
      if (localMkvCheck) localMkvCheck.error = String(e?.message || e)
    }
    localMkvChecking = false
  }

  function closeLocalMkvCheck() { localMkvCheck = null }

// --- Demandes de reseed (admin) ---
  let reqFilter = 'pending'        // 'pending' | 'done' | 'rejected' | 'all' | 'mine'
  let reqList = []                  // []ReseedRequest
  let reqLoading = false
  let reqError = ''
  let reqPage = 1
  let reqTotalPages = 1
  let reqProcessing = {}            // { [requestId]: true }
  let reqProgress = {}              // { [requestId]: { stage, msg, percent, speed_mb, bytes, total } }
  let reqActiveID = 0               // ID de la demande en cours (pour router les events)
  let myUserID = 550257             // TODO: récupérer via GetMyUsername/user-profile/me

  async function loadReseedRequests() {
    reqLoading = true
    reqError = ''
    try {
      const status = (reqFilter === 'all' || reqFilter === 'mine') ? '' : reqFilter
      const uploader = reqFilter === 'mine' ? myUserID : 0
      const r = await ListReseedRequests(status, uploader, 0, reqPage)
      reqList = r?.pagination?.data || []
      reqTotalPages = r?.pagination?.last_page || 1
    } catch(e) {
      reqError = String(e?.message || e)
      addLog('REQ', '✗ ' + reqError)
    }
    reqLoading = false
  }

  $: if (activeTab === 'reseed' && reseedSubTab === 'requests') loadReseedRequests()
  // Reset page quand filtre change
  $: if (reqFilter) { reqPage = 1 }

  async function processReseedRequest(req) {
    if (!req?.torrent?.title_id || !req?.torrent_id) { addLog('REQ', '⚠ request #' + req.id + ' : title_id/torrent_id manquant'); return }
    reqProcessing = { ...reqProcessing, [req.id]: true }
    reqActiveID = req.id
    reqProgress = { ...reqProgress, [req.id]: { stage: 'starting', msg: 'Démarrage…', percent: 0, speed_mb: 0, bytes: 0, total: 0 } }
    addLog('REQ', `▶ Traitement request #${req.id} → fiche #${req.torrent.title_id} torrent #${req.torrent_id}`)
    try {
      const saison = parseInt(req.torrent.torrent_name?.match(/[sS](\d{1,2})[eE]/)?.[1] || '0') || 0
      const ep = parseInt(req.torrent.torrent_name?.match(/[sS]\d{1,2}[eE](\d{1,3})/)?.[1] || '0') || 0
      // Workflow complet : DDL 1fichier → FTP (avec nom exact du torrent) +
      // push .torrent seedbox + force recheck. Nécessite FTP + seedbox + 1fichier.
      const r = await AutoReseedFullFromTorrent(req.torrent_id, req.torrent.title_id, saison, ep)
      addLog('REQ', `✓ Request #${req.id} : FTP ${r.expected_filename} · seedbox ${r.seedbox_path}${r.rechecked ? ' · recheck OK' : ' · recheck à refaire manuellement'}`)
      reqProgress = { ...reqProgress, [req.id]: { ...reqProgress[req.id], stage: 'done', msg: '✓ Terminé', percent: 100 } }
      try { Notify('✓ Reseed demandé traité', req.torrent.torrent_name) } catch(e) {}
    } catch(e) {
      // Fallback push torrent seul si workflow complet échoue
      addLog('REQ', `⚠ Reseed complet #${req.id} échoué (${e}) — fallback push torrent seul`)
      try {
        const saison = parseInt(req.torrent.torrent_name?.match(/[sS](\d{1,2})[eE]/)?.[1] || '0') || 0
        const ep = parseInt(req.torrent.torrent_name?.match(/[sS]\d{1,2}[eE](\d{1,3})/)?.[1] || '0') || 0
        const r2 = await AutoReseedFromHydracker(req.torrent.title_id, saison, ep, 0, 0)
        addLog('REQ', `✓ Fallback request #${req.id} : torrent #${r2.torrent_id} → seedbox`)
        reqProgress = { ...reqProgress, [req.id]: { ...reqProgress[req.id], stage: 'done', msg: '✓ Fallback seedbox OK', percent: 100 } }
      } catch(e2) {
        addLog('REQ', `✗ Request #${req.id} : ${e2}`)
        reqProgress = { ...reqProgress, [req.id]: { ...reqProgress[req.id], stage: 'error', msg: '✗ ' + String(e2) } }
      }
    }
    reqProcessing = { ...reqProcessing, [req.id]: false }
    if (reqActiveID === req.id) reqActiveID = 0
  }

  async function processAllPending() {
    const pending = reqList.filter(r => r.status === 'pending')
    addLog('REQ', `▶ Batch : ${pending.length} demande(s)`)
    for (const r of pending) {
      await processReseedRequest(r)
    }
    addLog('REQ', '✓ Batch terminé')
  }

// --- Mes uploads (admin CRUD sur tes propres items) ---
  // myUsername vient de l'auth team (résolu via /user-profile/me)
  let myTab = 'liens'                      // 'liens' | 'nzbs' (torrents retirés)
  let myItems = []                         // []Lien ou []TorrentItem
  let myLoading = false
  let myError = ''
  let myPage = 1
  let myTotalPages = 1
  let editingItem = null                   // item en cours d'édition (modal)
  let editPayload = { quality: 0, lang: 0, saison: 0, episode: 0, active: -1 }
  let deletingItem = null                  // item dont on demande confirmation delete
  let deleteConfirmInput = ''              // texte tapé par user pour confirmer
  let qualityOptions = []

  async function loadMyUploads() {
    myLoading = true
    myError = ''
    myItems = []
    try {
      if (myTab === 'liens') {
        const r = await ListMyLiens(myUsername, myPage)
        myItems = r?.pagination?.data || []
        myTotalPages = r?.pagination?.last_page || 1
      } else if (myTab === 'nzbs') {
        const r = await ListMyNzbs(myUsername, myPage)
        myItems = r?.pagination?.data || []
        // AdminNzbsResponse n'a pas last_page — on utilise next_page + total pour estimer
        const total = r?.pagination?.total || 0
        const perPage = myItems.length || 20
        myTotalPages = total > 0 ? Math.ceil(total / perPage) : (r?.pagination?.next_page ? myPage + 1 : myPage)
      }
    } catch(e) {
      myError = String(e?.message || e)
      addLog('MY', '✗ ' + myError)
    }
    myLoading = false
  }

  $: if (activeTab === 'myuploads') { loadMyUploads(); if (!qualityOptions.length) GetMetaQualities().then(q => qualityOptions = q || []).catch(() => {}) }
  $: if (myTab) { myPage = 1 }

  function startEdit(item) {
    editingItem = item
    editPayload = {
      quality: item.qualite || item.quality || 0,
      lang: item.lang_id || item.lang || (item.langues_compact?.[0]?.id) || 0,
      saison: item.saison || item.season || 0,
      episode: item.episode || 0,
      active: item.active === 0 ? 0 : (item.active === 1 ? 1 : -1),
    }
  }
  function cancelEdit() { editingItem = null }

  async function saveEdit() {
    if (!editingItem) return
    const id = editingItem.id
    try {
      if (myTab === 'torrents') {
        await UpdateMyTorrent(id, editPayload.quality, editPayload.lang, editPayload.saison, editPayload.episode, editPayload.active)
      } else {
        await UpdateMyLien(id, editPayload.quality, editPayload.lang, editPayload.saison, editPayload.episode, editPayload.active)
      }
      addLog('MY', `✓ Modifié ${myTab.slice(0,-1)} #${id}`)
      try { Notify('✓ Modifié', `${myTab.slice(0,-1)} #${id}`) } catch(e) {}
      editingItem = null
      loadMyUploads()
    } catch(e) {
      addLog('MY', `✗ ${e}`)
    }
  }

  function startDelete(item) {
    deletingItem = item
    deleteConfirmInput = ''
  }
  function cancelDelete() { deletingItem = null; deleteConfirmInput = '' }

  async function confirmDelete() {
    if (!deletingItem) return
    // Vérif : l'user doit avoir tapé l'ID exact pour confirmer
    if (deleteConfirmInput.trim() !== String(deletingItem.id)) {
      addLog('MY', '⚠ Suppression annulée : ID ne correspond pas')
      return
    }
    const id = deletingItem.id
    try {
      if (myTab === 'liens') {
        await DeleteMyLien(id)
      } else if (myTab === 'nzbs') {
        await DeleteMyNzb(id)
      }
      addLog('MY', `✓ Supprimé ${myTab.slice(0,-1)} #${id}`)
      try { Notify('🗑 Supprimé', `${myTab.slice(0,-1)} #${id}`) } catch(e) {}
      deletingItem = null
      deleteConfirmInput = ''
      loadMyUploads()
    } catch(e) {
      addLog('MY', `✗ ${e}`)
    }
  }

  function qualityName(id) { return qualityOptions.find(q => q.id === id)?.name || ('qual#' + id) }

// --- Stats (global site + fiches + uploaders + user search) ---
  let statsLoading = false
  let statsGlobal = null       // { topTitles, liensSample, torrentsSample, uploaders, qualDist, hostDist }
  let statsError = ''

  // Recherche user : affiche profil + aperçu uploads
  let statsUserQuery = ''
  let statsUserProfile = null
  let statsUserLiens = []
  let statsUserTorrents = []
  let statsUserLoading = false
  let statsUserError = ''

  async function loadGlobalStats() {
    statsLoading = true
    statsError = ''
    try {
      // 1. Top fiches par popularité
      const topTitles = await ListTitlesSorted('popularity:desc', 20, 1).catch(() => null)
      // 2. Sample des 100 derniers liens + torrents pour stats distribution
      const liensSample = await ListMyLiens('', 1).catch(() => null)   // sans filtre user = tous
      const torrentsSample = await ListMyTorrents('', 1).catch(() => null)

      // Agrégation locale
      const liens = liensSample?.pagination?.data || []
      const torrents = torrentsSample?.pagination?.data || []

      // Top uploaders : count par id_user sur les samples combinés
      const uploaderCount = {}
      liens.forEach(l => {
        const u = l.id_user || '?'
        uploaderCount[u] = (uploaderCount[u] || 0) + 1
      })
      torrents.forEach(t => {
        const u = t.author || t.id_user || '?'
        uploaderCount[u] = (uploaderCount[u] || 0) + 1
      })
      const topUploaders = Object.entries(uploaderCount)
        .sort((a, b) => b[1] - a[1])
        .slice(0, 15)
        .map(([username, count]) => ({ username, count }))

      // Répartition qualité (liens + torrents)
      const qualCount = {}
      ;[...liens, ...torrents].forEach(item => {
        const q = item.qualite || item.quality || 0
        if (q) qualCount[q] = (qualCount[q] || 0) + 1
      })
      const qualDist = Object.entries(qualCount)
        .sort((a, b) => b[1] - a[1])
        .map(([qid, count]) => ({ id: parseInt(qid), name: qualityName(parseInt(qid)), count }))

      // Répartition hosts (liens only)
      const hostCount = {}
      liens.forEach(l => {
        const h = l.host?.name || ('#' + (l.id_host || '?'))
        hostCount[h] = (hostCount[h] || 0) + 1
      })
      const hostDist = Object.entries(hostCount)
        .sort((a, b) => b[1] - a[1])
        .map(([name, count]) => ({ name, count }))

      statsGlobal = {
        topTitles: topTitles?.data || [],
        totalTitles: topTitles?.pagination?.total || 0,
        totalLiens: liensSample?.pagination?.total || 0,
        totalTorrents: torrentsSample?.pagination?.total || 0,
        liensCount: liens.length,
        torrentsCount: torrents.length,
        topUploaders,
        qualDist,
        hostDist,
      }
    } catch(e) {
      statsError = String(e?.message || e)
    }
    statsLoading = false
  }

  async function searchStatsUser() {
    statsUserError = ''
    statsUserProfile = null
    statsUserLiens = []
    statsUserTorrents = []
    const q = (statsUserQuery || '').trim()
    if (!q) return
    statsUserLoading = true
    try {
      statsUserProfile = await GetUserProfile(q)
      // Aperçu uploads du user
      const [liens, torrents] = await Promise.all([
        ListMyLiens(q, 1).catch(() => null),
        ListMyTorrents(q, 1).catch(() => null),
      ])
      statsUserLiens = liens?.pagination?.data || []
      statsUserTorrents = torrents?.pagination?.data || []
      statsUserProfile._totalLiens = liens?.pagination?.total || 0
      statsUserProfile._totalTorrents = torrents?.pagination?.total || 0
    } catch(e) {
      statsUserError = String(e?.message || e)
    }
    statsUserLoading = false
  }

  $: if (activeTab === 'stats' && !statsGlobal) loadGlobalStats()
  $: if (activeTab === 'stats' && !qualityOptions.length) GetMetaQualities().then(q => qualityOptions = q || []).catch(() => {})

  // --- Stats uploaders (page d'accueil Stats) ---
  let topUploaders = null          // UploaderScanResult
  let topUploadersLoading = false
  let topUploadersError = ''
  let scanSize = 30  // jours à scanner (au lieu de "fiches")
  let uploadersSort = 'total'      // 'total' | 'torrents' | 'nzbs' | 'liens' | 'size' | 'last' | 'author'
  let uploadersSortDir = 'desc'    // 'desc' | 'asc'
  let uploadersFilter = ''
  async function loadTopUploaders() {
    topUploadersLoading = true
    topUploadersError = ''
    try {
      topUploaders = await GetUploaderStats(scanSize)
    } catch(e) {
      topUploadersError = String(e?.message || e)
    }
    topUploadersLoading = false
  }
  function setSort(col) {
    if (uploadersSort === col) { uploadersSortDir = uploadersSortDir === 'desc' ? 'asc' : 'desc' }
    else { uploadersSort = col; uploadersSortDir = 'desc' }
  }
  $: filteredUploaders = (() => {
    if (!topUploaders?.uploaders) return []
    const q = uploadersFilter.trim().toLowerCase()
    let list = q ? topUploaders.uploaders.filter(u => u.author.toLowerCase().includes(q)) : [...topUploaders.uploaders]
    const dir = uploadersSortDir === 'desc' ? -1 : 1
    const cmp = {
      total:    (a,b) => (a.total - b.total) * dir,
      torrents: (a,b) => (a.torrents - b.torrents) * dir,
      nzbs:     (a,b) => (a.nzbs - b.nzbs) * dir,
      liens:    (a,b) => (a.liens - b.liens) * dir,
      size:     (a,b) => (a.total_size - b.total_size) * dir,
      last:     (a,b) => ((a.last_upload_at||'') > (b.last_upload_at||'') ? 1 : -1) * dir,
      author:   (a,b) => a.author.localeCompare(b.author) * dir,
    }[uploadersSort]
    if (cmp) list.sort(cmp)
    return list
  })()

  function fmtBytes(n) {
    if (!n) return '0'
    if (n > 1e12) return (n/1e12).toFixed(2) + ' TB'
    if (n > 1e9) return (n/1e9).toFixed(2) + ' GB'
    if (n > 1e6) return (n/1e6).toFixed(2) + ' MB'
    return n + ' B'
  }

// --- Log API : capture les entrées api:log émises par le client Go ---
  let apiLogs = []                  // []APILogEntry (max 500 entries, FIFO)
  let apiLogFilter = 'all'          // 'all' | 'ok' | 'error'
  let apiLogSearch = ''
  let apiLogExpanded = {}           // { [idx]: true } pour afficher le body_preview
  const API_LOG_MAX = 500

  function clearApiLogs() { apiLogs = []; apiLogExpanded = {} }

  $: filteredApiLogs = apiLogs.filter(e => {
    if (apiLogFilter === 'ok' && (e.error || e.status >= 400)) return false
    if (apiLogFilter === 'error' && !e.error && e.status < 400) return false
    if (apiLogSearch) {
      const q = apiLogSearch.toLowerCase()
      if (!e.url?.toLowerCase().includes(q) && !e.body_preview?.toLowerCase().includes(q) && !e.error?.toLowerCase().includes(q)) return false
    }
    return true
  })

// Check Torrent
  let checkTorrents = []
  let checkLoading = false
  let checkState = {}  // { [hash]: { stage, msg, percent, speed } }
  let checkFilter = 'all'  // 'all' | 'active' | 'inactive'
  let checkSort = 'seeds_desc'
  let checkPage = 1
  const checkPageSize = 100
  let checkSelected = new Set() // hashes sélectionnés
  let checkExpanded = ''        // hash du torrent expandé (un seul à la fois)
  let checkDeleting = false
  let checkDeleteResult = ''    // message après bulk delete
  let seedboxSectionOpen = false  // Ma seedbox repliée par défaut
  let seedboxLocalSearch = {}    // { [hash]: { state: 'searching|ok|err|nomatch', results?, error? } }
  function isIncomplete(t) { return t.size > 0 && t.done < t.size }
  function seedsOf(t) {
    const v = t.seeders ?? t.seeds ?? t.seed ?? t.num_seeders
    return v == null ? 0 : parseInt(v) || 0
  }
  $: filteredCheckTorrents = (() => {
    let arr = checkTorrents.filter(t => {
      if (checkFilter === 'active')   return t.is_active === 1
      if (checkFilter === 'inactive') return isIncomplete(t)
      return true
    })
    const cmp = {
      seeds_desc: (a, b) => seedsOf(b) - seedsOf(a),
      seeds_asc:  (a, b) => seedsOf(a) - seedsOf(b),
      size_desc:  (a, b) => (b.size || 0) - (a.size || 0),
      name:       (a, b) => (a.file_name || a.name || '').localeCompare(b.file_name || b.name || ''),
    }[checkSort] || (() => 0)
    return [...arr].sort(cmp)
  })()
  $: checkTotalPages = Math.max(1, Math.ceil(filteredCheckTorrents.length / checkPageSize))
  $: pagedCheckTorrents = filteredCheckTorrents.slice((checkPage - 1) * checkPageSize, checkPage * checkPageSize)
  $: if (checkPage > checkTotalPages) checkPage = checkTotalPages
  // Reset page sur changement de filtre ou tri
  $: if (checkFilter || checkSort) checkPage = 1

  async function loadCheckTorrents(refresh = false) {
    checkLoading = true
    try { checkTorrents = await ListCheckTorrents(refresh) || [] }
    catch(e) { addLog('CHK', '✗ ' + e) }
    checkLoading = false
  }
  async function reseedOne(t) {
    if (!t.lihdl_url || !t.file_name) return
    checkState = { ...checkState, [t.hash]: { stage: 'download', msg: 'Démarrage…', percent: 0, speed: 0 } }
    try { await ReseedFromLihdl(t.hash, t.lihdl_url, t.file_name) }
    catch(e) { addLog('CHK', '✗ ' + e) }
  }

  function toggleCheckSelect(hash) {
    if (checkSelected.has(hash)) checkSelected.delete(hash)
    else checkSelected.add(hash)
    checkSelected = new Set(checkSelected) // trigger Svelte reactivity
  }
  function toggleCheckSelectAllPaged() {
    const allSelected = pagedCheckTorrents.every(t => checkSelected.has(t.hash))
    if (allSelected) {
      pagedCheckTorrents.forEach(t => checkSelected.delete(t.hash))
    } else {
      pagedCheckTorrents.forEach(t => checkSelected.add(t.hash))
    }
    checkSelected = new Set(checkSelected)
  }
  function clearCheckSelection() {
    checkSelected = new Set()
  }
  $: checkSelectedSize = pagedCheckTorrents
    .concat(filteredCheckTorrents)
    .filter((t, i, arr) => arr.findIndex(x => x.hash === t.hash) === i) // dedupe
    .filter(t => checkSelected.has(t.hash))
    .reduce((acc, t) => acc + (t.size || 0), 0)
  $: pagedAllSelected = pagedCheckTorrents.length > 0 && pagedCheckTorrents.every(t => checkSelected.has(t.hash))

  async function bulkDeleteSelected() {
    const hashes = [...checkSelected]
    if (!hashes.length) return
    // Récupère les torrents complets correspondants (avec ID Hydracker)
    const targets = checkTorrents.filter(t => checkSelected.has(t.hash))
    const total = targets.length
    if (!total) return
    if (!confirm(`Supprimer ${total} torrent(s) : .torrent de la seedbox + .mkv sur FTP ?\n(Le post Hydracker reste intact)`)) return
    checkDeleting = true
    checkDeleteResult = ''
    let ok = 0, errs = []
    for (const t of targets) {
      try {
        if (!t.id) throw new Error('ID Hydracker manquant')
        await DeleteTorrentAndFTP(t.id)
        ok++
        checkTorrents = checkTorrents.filter(x => x.hash !== t.hash)
        checkSelected.delete(t.hash)
      } catch (e) {
        errs.push(`${(t.torrent_name || t.name || '#'+t.id).slice(0,40)}: ${String(e?.message || e).replace(/^Error:\s*/, '')}`)
      }
    }
    checkSelected = new Set(checkSelected)
    checkDeleting = false
    checkDeleteResult = `✅ ${ok}/${total} supprimé(s)${errs.length ? ' · ❌ ' + errs.length + ' erreur(s)' : ''}`
    addLog('CHK', `Bulk delete : ${ok}/${total} OK${errs.length ? ` · errs: ${errs.join(' | ')}` : ''}`)
  }

  async function browseLocalForRow(t) {
    // Ouvre le modal "Parcourir un MKV local" avec un préfill éventuel
    await checkLocalMkv()
  }

  async function searchFreeLink(t) {
    const q = t.file_name || t.name || ''
    if (!q) return
    seedboxLocalSearch = { ...seedboxLocalSearch, [t.hash]: { state: 'searching' } }
    try {
      const results = await MediaSearch(q)
      if (results && results.length > 0) {
        seedboxLocalSearch = { ...seedboxLocalSearch, [t.hash]: { state: 'ok', results } }
      } else {
        seedboxLocalSearch = { ...seedboxLocalSearch, [t.hash]: { state: 'nomatch' } }
      }
    } catch (e) {
      seedboxLocalSearch = { ...seedboxLocalSearch, [t.hash]: { state: 'err', error: String(e?.message || e).replace(/^Error:\s*/, '') } }
    }
  }

  async function deleteOne(hash) {
    if (!confirm('Supprimer ce torrent de la seedbox ?')) return
    try {
      await DeleteSeedboxByHash(hash)
      checkTorrents = checkTorrents.filter(t => t.hash !== hash)
      checkSelected.delete(hash)
      checkSelected = new Set(checkSelected)
    } catch (e) {
      addLog('CHK', '✗ delete : ' + e)
    }
  }

  onMount(async () => {
    try {
      const loaded = await GetConfig()
      if (loaded) cfg = { ...cfg, ...loaded }
    } catch {}
    try { appVersion = await GetVersion() } catch {}
    try { updateInfo = await CheckForUpdate() } catch {}
    // Identité : on interroge Hydracker via son token bearer pour récupérer le
    // pseudo (sert au filtre "Mes uploads"). Si pas de token / pas de réponse,
    // myUsername reste vide → "Mes uploads" affiche tous les uploads.
    try {
      const me = await GetUserProfile('me')
      if (me && me.name) {
        myUsername = me.name
        fetchAvatar()
      }
    } catch {}
    checkLihdlPasswordStatus()
    checkSeedboxPasswordStatus()
    try { hydrackerURLManaged = await IsHydrackerURLManaged() } catch {}
    if (hydrackerURLManaged) {
      try { cfg.hydracker_base_url = await GetEffectiveHydrackerURL() } catch {}
    }
    EventsOn('watch:status', s => { watchRunning = !!s.running; if (s.running) addLog('WATCH', `Surveillance active : ${s.folder}`) })
    EventsOn('update:progress', p => { updateState = { ...updateState, stage: p.stage || '', msg: p.msg || '', percent: p.percent || 0 } })
    EventsOn('watch:newfile', path => {
      addLog('WATCH', `📥 Nouveau fichier : ${path}`)
      window.dispatchEvent(new CustomEvent('watch:newfile', { detail: path }))
    })
    // Démarrage auto si configuré
    if (cfg.watch_auto_start && cfg.watch_folder) {
      try { await StartWatchFolder(cfg.watch_folder) } catch(e) { addLog('WATCH', '✗ ' + e) }
    }
    EventsOn('nzb:status', s => {
      nzbStatus = s
      if (s === 'Terminé') nzbDone = true
      addLog('NZB', s)
    })
    EventsOn('nzb:parpar', p => { if (p.percent !== undefined) nzbParparPct = p.percent })
    EventsOn('nzb:nyuu',   p => {
      if (p.percent  !== undefined) nzbNyuuPct     = p.percent
      if (p.speed    !== undefined) nzbNyuuSpeed   = p.speed
      if (p.eta      !== undefined) nzbNyuuETA     = p.eta
      if (p.articles !== undefined) nzbNyuuArticles = p.articles
    })
    EventsOn('nzb:result', r => { nzbResult = r; nzbDone = true; addLog('NZB', r.message) })
    EventsOn('ddl:log',    msg => addLog('DDL', msg))
    EventsOn('check:log',  msg => addLog('CHK', msg))
    EventsOn('check:status', p => {
      if (!p.hash) return
      checkState = { ...checkState, [p.hash]: { ...(checkState[p.hash] || {}), stage: p.stage, msg: p.msg } }
      addLog('CHK', p.msg)
    })
    EventsOn('check:progress', p => {
      if (!p.hash) return
      checkState = { ...checkState, [p.hash]: { ...(checkState[p.hash] || {}), percent: p.percent ?? 0, speed: p.speed ?? 0 } }
    })
    EventsOn('reseed:status', p => {
      reseedStage = p.stage || ''
      reseedMsg = p.msg || ''
      addLog('RES', p.msg || p.stage)
    })
    EventsOn('autoreseed:status', p => {
      autoReseedStatus = p.msg || p.stage || ''
      if (p.msg) addLog('AUTO', p.msg)
      // Route l'état vers le torrent actif de l'onglet Check Torrent
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || {}),
          stage: p.stage || 'progress',
          msg: p.msg || '',
        } }
      }
    })
    EventsOn('autoreseed:progress', p => {
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || { stage: 'progress', msg: '' }),
          percent: p.percent || 0,
          speed_mb: p.speed_mb || 0,
        } }
      }
    })
    EventsOn('autoreseed_ddl:status', p => {
      autoReseedStatus = p.msg || p.stage || ''
      if (p.msg) addLog('AUTO', p.msg)
    })
    EventsOn('autoreseed_ddl:progress', p => {
      autoReseedDDLProgress = {
        percent: p.percent || 0,
        speed_mb: p.speed_mb || 0,
        bytes: p.bytes || 0,
        total: p.total || 0,
      }
    })
    EventsOn('api:log', entry => {
      apiLogs = [entry, ...apiLogs].slice(0, API_LOG_MAX)
    })
    // Events du workflow complet (DDL→FTP+torrent+recheck) — on les route vers
    // la demande active (reqActiveID) pour afficher barre + vitesse dans le UI.
    EventsOn('autoreseed_full:status', p => {
      if (reqActiveID) {
        reqProgress = { ...reqProgress, [reqActiveID]: { ...(reqProgress[reqActiveID] || {}), stage: p.stage || '', msg: p.msg || '' } }
      }
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || {}),
          stage: p.stage || 'progress',
          msg: p.msg || '',
        } }
      }
      if (p.msg) addLog('AUTO', p.msg)
    })
    EventsOn('autoreseed_full:progress', p => {
      if (reqActiveID) {
        reqProgress = { ...reqProgress, [reqActiveID]: {
          ...(reqProgress[reqActiveID] || {}),
          stage: 'ftp',
          percent: p.percent || 0,
          speed_mb: p.speed_mb || 0,
          bytes: p.bytes || 0,
          total: p.total || 0,
        } }
      }
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || { stage: 'ftp', msg: '' }),
          percent: p.percent || 0,
          speed_mb: p.speed_mb || 0,
        } }
      }
    })
    EventsOn('autoreseed_full:seedbox', p => {
      if (reqActiveID) {
        reqProgress = { ...reqProgress, [reqActiveID]: {
          ...(reqProgress[reqActiveID] || {}),
          stage: 'seedbox',
          msg: `Seedbox : ${(p.percent || 0).toFixed(0)}%`,
        } }
      }
      if (checkActionActiveID) {
        checkActionProg = { ...checkActionProg, [checkActionActiveID]: {
          ...(checkActionProg[checkActionActiveID] || {}),
          stage: 'seedbox',
          percent: p.percent || 0,
          speed_mb: p.speed_mb || 0,
          msg: `Seedbox : ${(p.percent || 0).toFixed(0)}%`,
        } }
      }
    })
    EventsOn('reseed:progress', p => {
      reseedPct = p.percent ?? 0
      reseedSpeed = p.speed_mb ?? 0
    })
  })

  onDestroy(() => EventsOff('nzb:status', 'nzb:parpar', 'nzb:nyuu', 'nzb:result', 'ddl:log'))

  async function saveConfig() {
    await SaveConfig(cfg)
    cfgSaved = true
    setTimeout(() => cfgSaved = false, 2000)
  }

  // --- Tests ---
  let testResults = {}
  let testLoading = {}

  async function runTest(key, fn) {
    testLoading[key] = true
    testLoading = testLoading
    try {
      testResults[key] = await fn()
    } catch(e) {
      testResults[key] = { ok: false, message: e.toString() }
    }
    testLoading[key] = false
    testLoading = testLoading
    testResults = testResults
  }
</script>

{#if authState === 'login'}
  <div class="auth-screen">
    <div class="auth-card">
      <img src={loginLogo} alt="" class="auth-logo" />
      <div style="font-size:20px;font-weight:700;letter-spacing:0.5px;margin-bottom:4px">GO Post Tools</div>
      <div style="color:var(--text3);font-size:12px;margin-bottom:24px;letter-spacing:2px;text-transform:uppercase">Connexion</div>
      <form on:submit|preventDefault={doLogin} style="display:flex;flex-direction:column;gap:10px;text-align:left">
        <div class="field">
          <label for="login-pseudo">Pseudo</label>
          <input id="login-pseudo" type="text" bind:value={loginPseudo}
            autocomplete="username" autocapitalize="off" spellcheck="false"
            placeholder="Ton pseudo" disabled={loginLoading} />
        </div>
        <div class="field">
          <label for="login-password">Mot de passe</label>
          <input id="login-password" type="password" bind:value={loginPassword}
            autocomplete="current-password"
            placeholder="••••••••" disabled={loginLoading} />
        </div>
        {#if loginError}
          <div style="color:#ff9585;font-size:12px;background:rgba(255,68,68,0.08);border:1px solid rgba(255,68,68,0.3);padding:8px 10px;border-radius:8px">
            ⚠ {loginError}
          </div>
        {/if}
        <button class="btn-save" type="submit" disabled={loginLoading || !loginPseudo || !loginPassword} style="margin-top:6px">
          {loginLoading ? '…' : '🔐 Se connecter'}
        </button>
      </form>
      <div style="color:var(--text3);font-size:11px;margin-top:16px;line-height:1.5">
        Pas de compte ? Contacte <b>Gandalf</b>.
      </div>
    </div>
  </div>
{:else}
<div class="layout">
  <!-- Sidebar -->
  <aside class="sidebar" class:collapsed={sidebarCollapsed}>
    <button class="sidebar-toggle" on:click={() => sidebarCollapsed = !sidebarCollapsed} title={sidebarCollapsed ? 'Déplier la barre' : 'Replier la barre'}>
      {sidebarCollapsed ? '›' : '‹'}
    </button>
    <div class="brand">
      <img src={logo} alt="" class="brand-logo" />
      {#if !sidebarCollapsed}
        <div class="logo">GO Post Tools</div>
        {#if appVersion}<div class="brand-version">v{appVersion}</div>{/if}
        <div class="brand-author">By GANDALF</div>
      {/if}
    </div>

    <!-- Carte user retirée : plus d'auth (identité via token Hydracker) -->
    {#if myUsername && !sidebarCollapsed}
      <div style="text-align:center;font-size:11px;color:var(--text3);margin-bottom:8px">
        Connecté : <b style="color:var(--text2)">{myUsername}</b>
      </div>
    {/if}

    <nav>
      {#each visibleTabs as tab}
        <button class="nav-item" class:active={activeTab === tab.id}
          on:click={() => activeTab = tab.id} title={sidebarCollapsed ? tab.label : ''}>
          {sidebarCollapsed ? tab.label.split(' ')[0] : tab.label}
        </button>
      {/each}
    </nav>
    <div class="sidebar-footer">
      {#if updateInfo?.available}
        <button class="btn-update" on:click={() => showUpdateModal = true} title="Télécharger la nouvelle version">
          {sidebarCollapsed ? '🆕' : `🆕 Mise à jour v${updateInfo.latest}`}
        </button>
      {:else}
        <button class="btn-check-update" on:click={recheckUpdate} disabled={updateChecking} title="Vérifier les mises à jour">
          {#if updateChecking}⌛{:else}🔄{/if}
          {#if !sidebarCollapsed}<span>{updateCheckMsg || 'Vérifier maj'}</span>{/if}
        </button>
      {/if}
      <!-- Bouton logout retiré : plus d'auth -->
    </div>
  </aside>

  <!-- Main content -->
  <main class="content">

    <!-- HydrackerTab toujours monté pour préserver l'état -->
    <div style="display:{activeTab === 'hydracker' ? 'contents' : 'none'}">
      <HydrackerTab />
    </div>

    {#if activeTab !== 'hydracker'}

    <!-- ===== FICHES ===== -->
    {#if activeTab === 'fiches'}
      <div class="tab-content">
        <h2>🎞 Fiches Hydracker</h2>

        {#if !fichesSelected}
          <!-- Mode recherche -->
          <div class="section">
            <div class="section-header"><span>Recherche</span></div>
            <div class="field" style="display:flex;gap:6px;margin-bottom:10px">
              <button class="btn-test" class:active-chip={fichesMode === 'name'} on:click={() => fichesMode = 'name'}>🔤 Par nom</button>
              <button class="btn-test" class:active-chip={fichesMode === 'hydracker_id'} on:click={() => fichesMode = 'hydracker_id'}>🆔 ID Hydracker</button>
              <button class="btn-test" class:active-chip={fichesMode === 'tmdb_id'} on:click={() => fichesMode = 'tmdb_id'}>🎬 ID TMDB</button>
            </div>
            <div class="field">
              <div class="pwd-row">
                <input type="text" bind:value={fichesQuery}
                  placeholder={fichesMode === 'name' ? 'Titre du film/série…' : fichesMode === 'hydracker_id' ? 'Ex: 62544' : 'Ex: 550 (Fight Club)'}
                  on:keydown={e => e.key === 'Enter' && !fichesLoading && fichesSearch()} />
                <button class="btn-save" on:click={fichesSearch} disabled={fichesLoading || !fichesQuery}>
                  {fichesLoading ? '…' : '🔍 Chercher'}
                </button>
              </div>
            </div>
            {#if fichesError}
              <div style="color:#ff9585;font-size:12px;margin-top:6px">⚠ {fichesError}</div>
            {/if}
          </div>

          <!-- Résultats en grille -->
          {#if fichesResults.length}
            <div class="section">
              <div class="section-header"><span>Résultats ({fichesResults.length})</span></div>
              <div class="fiches-grid">
                {#each fichesResults as f}
                  <button class="fiche-card" on:click={() => fichesOpen(f)} title={f.name}>
                    {#if f.poster}
                      <img src={f.poster} alt={f.name} loading="lazy" />
                    {:else}
                      <div class="fiche-no-poster">📽</div>
                    {/if}
                    <div class="fiche-info">
                      <div class="fiche-name">{f.name}</div>
                      <div class="fiche-meta">
                        {f.type === 'series' ? '📺 Série' : '🎬 Film'}
                        {#if f.release_date}· {f.release_date.slice(0,4)}{/if}
                        {#if f.score}· ⭐ {f.score.toFixed(1)}{/if}
                      </div>
                      <div class="fiche-id">#{f.id}</div>
                    </div>
                  </button>
                {/each}
              </div>
            </div>
          {/if}
        {:else}
          <!-- Détail d'une fiche -->
          <div class="section">
            <div style="display:flex;gap:16px;align-items:flex-start">
              <button class="btn-test" on:click={fichesBackToResults}>← Retour</button>
              {#if fichesSelected.poster}
                <img src={fichesSelected.poster} alt={fichesSelected.name} style="width:120px;border-radius:8px" />
              {/if}
              <div style="flex:1">
                <h3 style="margin:0 0 6px">{fichesSelected.name}</h3>
                <div style="color:var(--text3);font-size:12px;margin-bottom:8px">
                  {fichesSelected.type === 'series' ? '📺 Série' : '🎬 Film'}
                  {#if fichesSelected.release_date}· {fichesSelected.release_date.slice(0,4)}{/if}
                  · Hydracker #{fichesSelected.id}
                  {#if fichesSelected.imdb_id}· IMDb {fichesSelected.imdb_id}{/if}
                </div>
                <!-- Notes (TMDB depuis Hydracker + IMDb depuis proxytmdb) -->
                <div style="display:flex;gap:14px;margin-bottom:10px;font-size:12px;color:var(--text2)">
                  {#if fichesSelected.score || fichesSelected.rating}
                    <span>⭐ <b>TMDB</b> {(fichesSelected.score || fichesSelected.rating).toFixed(1)}</span>
                  {/if}
                  {#if fichesImdbInfo?.note_imdb}
                    <span style="color:#ffd60a">🟡 <b>IMDb</b> {fichesImdbInfo.note_imdb} <span style="opacity:0.7">({fichesImdbInfo.vote_imdb?.toLocaleString() || ''} votes)</span></span>
                  {/if}
                </div>
                <button class="btn-test" on:click={() => OpenBrowser(`https://hydracker.com/titles/${fichesSelected.id}`)}>
                  🌐 Ouvrir sur Hydracker
                </button>
                {#if fichesSelected.imdb_id}
                  <button class="btn-test" on:click={() => OpenBrowser(`https://www.imdb.com/title/${fichesSelected.imdb_id}`)}>🟡 IMDb</button>
                {/if}
                {#if fichesSelected.tmdb_id}
                  <button class="btn-test" on:click={() => OpenBrowser(`https://www.themoviedb.org/${fichesSelected.type === 'series' ? 'tv' : 'movie'}/${fichesSelected.tmdb_id}`)}>🎬 TMDB</button>
                {/if}
              </div>
            </div>

            <!-- Watch providers FR (Netflix, Disney+, etc.) -->
            {#if fichesProviders && fichesProviders.FR}
              {@const fr = fichesProviders.FR}
              <div style="margin-top:14px;padding:10px;background:rgba(255,255,255,0.025);border-radius:8px">
                <div style="font-size:11px;color:var(--text3);margin-bottom:6px;text-transform:uppercase;letter-spacing:0.3px">📺 Streaming FR</div>
                <div style="display:flex;flex-wrap:wrap;gap:8px;align-items:center">
                  {#each (fr.flatrate || []) as p}
                    <div style="display:flex;align-items:center;gap:5px;padding:4px 8px;background:rgba(126,240,192,0.08);border:1px solid rgba(126,240,192,0.25);border-radius:6px;font-size:11px">
                      <img src={`https://image.tmdb.org/t/p/w45${p.logo_path}`} alt={p.provider_name} style="width:18px;height:18px;border-radius:3px" />
                      <span>{p.provider_name}</span>
                    </div>
                  {/each}
                  {#each (fr.rent || []) as p}
                    <div style="display:flex;align-items:center;gap:5px;padding:4px 8px;background:rgba(255,255,255,0.04);border:1px solid var(--border);border-radius:6px;font-size:11px;opacity:0.85" title="Location">
                      <img src={`https://image.tmdb.org/t/p/w45${p.logo_path}`} alt={p.provider_name} style="width:18px;height:18px;border-radius:3px" />
                      <span>{p.provider_name} <span style="color:var(--text3)">(loc)</span></span>
                    </div>
                  {/each}
                  {#if !(fr.flatrate?.length) && !(fr.rent?.length)}
                    <span style="color:var(--text3);font-size:11px">Pas dispo en streaming en France actuellement.</span>
                  {/if}
                </div>
              </div>
            {:else if fichesProvidersLoading}
              <div style="margin-top:10px;color:var(--text3);font-size:11px">⏳ Chargement des providers streaming…</div>
            {/if}
          </div>

          <div class="section">
            <div class="section-header"><span>Contenu</span></div>
            {#if fichesContentLoading}
              <div style="color:var(--text3);font-size:12px">Chargement…</div>
            {:else if fichesContent}
              <!-- Tabs : Torrents / NZB / Liens DDL -->
              <div class="field" style="display:flex;gap:6px;margin-bottom:12px">
                <button class="btn-test" class:active-chip={fichesContentTab === 'torrents'} on:click={() => fichesContentTab = 'torrents'}>
                  📦 Torrents ({fichesContent.torrents?.torrents?.length || 0})
                </button>
                <button class="btn-test" class:active-chip={fichesContentTab === 'nzbs'} on:click={() => fichesContentTab = 'nzbs'}>
                  📰 NZB ({fichesContent.nzbs?.nzbs?.length || 0})
                </button>
                <button class="btn-test" class:active-chip={fichesContentTab === 'liens'} on:click={() => fichesContentTab = 'liens'}>
                  🔗 Liens DDL ({fichesContent.liens?.liens?.length || 0})
                </button>
              </div>

              {#if fichesContentTab === 'torrents'}
                {#if fichesContent.torrents?.torrents?.length}
                  {#if fichesSelected.type === 'series'}
                    {#each groupBySeason(fichesContent.torrents.torrents) as sg (sg.season)}
                      <details class="season-group" open>
                        <summary class="season-summary">
                          <span class="season-chevron">▶</span>
                          <span class="season-title">📺 Saison {String(sg.season).padStart(2,'0')}</span>
                          <span class="season-count">{sg.items.length} torrent{sg.items.length > 1 ? 's' : ''}</span>
                        </summary>
                        <div class="content-grid">
                          {#each sg.items as t}
                            <div class="content-card">
                              <div class="cc-body">
                                <div class="cc-head">
                                  <span class="cc-id">#{t.id}</span>
                                  {#if t.qual?.qual}<span class="cc-chip cc-chip-qual">{t.qual.qual}</span>{/if}
                                  {#if t.saison || t.episode}<span class="cc-chip cc-chip-se">S{String(t.saison||0).padStart(2,'0')}E{String(t.episode||0).padStart(2,'0')}</span>{/if}
                                  {#if t.full_saison}<span class="cc-chip cc-chip-se">Saison complète</span>{/if}
                                  {#if t.size || t.taille}<span class="cc-chip cc-chip-size">{fmtSize(t.size || t.taille)}</span>{/if}
                                  {#if (t.seeders ?? null) !== null}
                                    <span class="cc-chip cc-chip-seed" style="color:{(t.seeders||0)===0?'#ff6b6b':((t.seeders||0)<=2?'#ffd60a':'#7ef0c0')}">🌱 {t.seeders||0}</span>
                                  {/if}
                                </div>
                                <div class="cc-name" title={t.torrent_name || t.name || ''}>{t.torrent_name || t.name || '(sans nom)'}</div>
                                <div class="cc-tags">
                                  {#each (t.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                                  {#each (t.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                                  {#if t.author}<span class="cc-tag cc-tag-author">👤 {t.author}</span>{/if}
                                </div>
                              </div>
                              <div class="cc-actions">
                                <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('torrents', t.id, t.torrent_name || t.name || '')}>ⓘ</button>
                                <button class="btn-test" on:click={() => downloadLienFromFiche({id:t.id, download_url:t.download_url, name:(t.torrent_name || t.name || ('torrent-'+t.id)) + '.torrent'})}>⬇ Torrent</button>
                              </div>
                            </div>
                          {/each}
                        </div>
                      </details>
                    {/each}
                  {:else}
                    <div class="content-grid">
                      {#each fichesContent.torrents.torrents as t}
                        <div class="content-card">
                          <div class="cc-body">
                            <div class="cc-head">
                              <span class="cc-id">#{t.id}</span>
                              {#if t.qual?.qual}<span class="cc-chip cc-chip-qual">{t.qual.qual}</span>{/if}
                              {#if t.saison || t.episode}<span class="cc-chip cc-chip-se">S{String(t.saison||0).padStart(2,'0')}E{String(t.episode||0).padStart(2,'0')}</span>{/if}
                              {#if t.full_saison}<span class="cc-chip cc-chip-se">Saison complète</span>{/if}
                              {#if t.size || t.taille}<span class="cc-chip cc-chip-size">{fmtSize(t.size || t.taille)}</span>{/if}
                              {#if (t.seeders ?? null) !== null}
                                <span class="cc-chip cc-chip-seed" style="color:{(t.seeders||0)===0?'#ff6b6b':((t.seeders||0)<=2?'#ffd60a':'#7ef0c0')}">🌱 {t.seeders||0}</span>
                              {/if}
                            </div>
                            <div class="cc-name" title={t.torrent_name || t.name || ''}>{t.torrent_name || t.name || '(sans nom)'}</div>
                            <div class="cc-tags">
                              {#each (t.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                              {#each (t.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                              {#if t.author}<span class="cc-tag cc-tag-author">👤 {t.author}</span>{/if}
                            </div>
                          </div>
                          <div class="cc-actions">
                            <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('torrents', t.id, t.torrent_name || t.name || '')}>ⓘ</button>
                            <button class="btn-test" on:click={() => downloadLienFromFiche({id:t.id, download_url:t.download_url, name:(t.torrent_name || t.name || ('torrent-'+t.id)) + '.torrent'})}>⬇ Torrent</button>
                          </div>
                        </div>
                      {/each}
                    </div>
                  {/if}
                {:else}
                  <div style="color:var(--text3);font-size:12px">Aucun torrent partagé via API pour cette fiche.</div>
                {/if}
              {:else if fichesContentTab === 'nzbs'}
                {#if fichesContent.nzbs?.nzbs?.length}
                  {#if fichesSelected.type === 'series'}
                    {#each groupBySeason(fichesContent.nzbs.nzbs) as sg (sg.season)}
                      <details class="season-group" open>
                        <summary class="season-summary">
                          <span class="season-chevron">▶</span>
                          <span class="season-title">📺 Saison {String(sg.season).padStart(2,'0')}</span>
                          <span class="season-count">{sg.items.length} NZB</span>
                        </summary>
                        <div class="content-grid">
                          {#each sg.items as n}
                            <div class="content-card">
                              <div class="cc-body">
                                <div class="cc-head">
                                  <span class="cc-id">#{n.id}</span>
                                  {#if n.qual?.qual}<span class="cc-chip cc-chip-qual">{n.qual.qual}</span>{/if}
                                  {#if n.saison || n.episode}<span class="cc-chip cc-chip-se">S{String(n.saison||0).padStart(2,'0')}E{String(n.episode||0).padStart(2,'0')}</span>{/if}
                                  {#if n.size || n.taille}<span class="cc-chip cc-chip-size">{fmtSize(n.size || n.taille)}</span>{/if}
                                </div>
                                <div class="cc-name" title={n.name || ''}>{n.name || '(sans nom)'}</div>
                                {#if nzbFilenames[n.id]?.state === 'ok' && nzbFilenames[n.id].files?.length}
                                  {@const main = nzbFilenames[n.id].files.find(f => /\.(mkv|mp4|avi)$/i.test(f.filename)) || nzbFilenames[n.id].files[0]}
                                  <div class="cc-name cc-name-mono cc-sub-url" title={main.filename}>📁 {main.filename}</div>
                                  {#if nzbFilenames[n.id].files.length > 1}
                                    <div class="cc-name-loading">+ {nzbFilenames[n.id].files.length - 1} autre(s) fichier(s)</div>
                                  {/if}
                                {:else if nzbFilenames[n.id]?.state === 'loading'}
                                  <div class="cc-name-loading">⏳ Lecture du NZB…</div>
                                {/if}
                                <div class="cc-tags">
                                  {#each (n.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                                  {#each (n.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                                  {#if n.author || n.id_user}<span class="cc-tag cc-tag-author">👤 {n.author || n.id_user}</span>{/if}
                                </div>
                              </div>
                              <div class="cc-actions">
                                <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('nzbs', n.id, n.name || '')}>ⓘ</button>
                                <button class="btn-test" on:click={() => downloadLienFromFiche({id:n.id, download_url:n.download_url, name:(n.name || ('nzb-'+n.id)) + '.nzb'})}>⬇ NZB</button>
                              </div>
                            </div>
                          {/each}
                        </div>
                      </details>
                    {/each}
                  {:else}
                    <div class="content-grid">
                      {#each fichesContent.nzbs.nzbs as n}
                        <div class="content-card">
                          <div class="cc-body">
                            <div class="cc-head">
                              <span class="cc-id">#{n.id}</span>
                              {#if n.qual?.qual}<span class="cc-chip cc-chip-qual">{n.qual.qual}</span>{/if}
                              {#if n.saison || n.episode}<span class="cc-chip cc-chip-se">S{String(n.saison||0).padStart(2,'0')}E{String(n.episode||0).padStart(2,'0')}</span>{/if}
                              {#if n.size || n.taille}<span class="cc-chip cc-chip-size">{fmtSize(n.size || n.taille)}</span>{/if}
                            </div>
                            <div class="cc-name" title={n.name || ''}>{n.name || '(sans nom)'}</div>
                            {#if nzbFilenames[n.id]?.state === 'ok' && nzbFilenames[n.id].files?.length}
                              {@const main = nzbFilenames[n.id].files.find(f => /\.(mkv|mp4|avi)$/i.test(f.filename)) || nzbFilenames[n.id].files[0]}
                              <div class="cc-name cc-name-mono cc-sub-url" title={main.filename}>📁 {main.filename}</div>
                              {#if nzbFilenames[n.id].files.length > 1}
                                <div class="cc-name-loading">+ {nzbFilenames[n.id].files.length - 1} autre(s) fichier(s)</div>
                              {/if}
                            {:else if nzbFilenames[n.id]?.state === 'loading'}
                              <div class="cc-name-loading">⏳ Lecture du NZB…</div>
                            {/if}
                            <div class="cc-tags">
                              {#each (n.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                              {#each (n.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                              {#if n.author || n.id_user}<span class="cc-tag cc-tag-author">👤 {n.author || n.id_user}</span>{/if}
                            </div>
                          </div>
                          <div class="cc-actions">
                            <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('nzbs', n.id, n.name || '')}>ⓘ</button>
                            <button class="btn-test" on:click={() => downloadLienFromFiche({id:n.id, download_url:n.download_url, name:(n.name || ('nzb-'+n.id)) + '.nzb'})}>⬇ NZB</button>
                          </div>
                        </div>
                      {/each}
                    </div>
                  {/if}
                {:else}
                  <div style="color:var(--text3);font-size:12px">Aucun NZB partagé via API pour cette fiche.</div>
                {/if}
              {:else if fichesContentTab === 'liens'}
                {#if fichesContent.liens?.liens?.length}
                  {#if fichesSelected.type === 'series'}
                    {#each groupBySeason(fichesContent.liens.liens) as sg (sg.season)}
                      <details class="season-group" open>
                        <summary class="season-summary">
                          <span class="season-chevron">▶</span>
                          <span class="season-title">📺 Saison {String(sg.season).padStart(2,'0')}</span>
                          <span class="season-count">{sg.items.length} DDL</span>
                        </summary>
                        <div class="content-grid">
                          {#each sg.items as l}
                            <div class="content-card">
                              <div class="cc-body">
                                <div class="cc-head">
                                  <span class="cc-id">#{l.id}</span>
                                  {#if l.qual?.qual}<span class="cc-chip cc-chip-qual">{l.qual.qual}</span>{/if}
                                  {#if l.saison || l.episode}<span class="cc-chip cc-chip-se">S{String(l.saison||0).padStart(2,'0')}E{String(l.episode||0).padStart(2,'0')}</span>{/if}
                                  {#if l.full_saison}<span class="cc-chip cc-chip-se">Saison complète</span>{/if}
                                  {#if l.taille}<span class="cc-chip cc-chip-size">{fmtSize(l.taille)}</span>{/if}
                                  {#if l.host?.name || l.id_host}<span class="cc-chip cc-chip-host">{l.host?.name || ('host#'+l.id_host)}</span>{/if}
                                </div>
                                {#if ddlFilenames[l.id]?.state === 'ok'}
                                  {#if ddlFilenames[l.id].filename}
                                    <div class="cc-name" title={ddlFilenames[l.id].filename}>{ddlFilenames[l.id].filename}</div>
                                  {/if}
                                  {#if ddlFilenames[l.id].url}
                                    <div class="cc-name cc-name-mono cc-sub-url" title={ddlFilenames[l.id].url}>{ddlFilenames[l.id].url}</div>
                                  {/if}
                                {:else if ddlFilenames[l.id]?.state === 'loading'}
                                  <div class="cc-name-loading">⏳ Débridage + résolution du nom…</div>
                                {:else if ddlFilenames[l.id]?.state === 'err'}
                                  <div class="cc-name-loading" style="color:#ff9585" title={ddlFilenames[l.id].error}>⚠ {ddlFilenames[l.id].error.length > 100 ? ddlFilenames[l.id].error.slice(0,100)+'…' : ddlFilenames[l.id].error}</div>
                                {:else}
                                  <div class="cc-name-loading">⏳ En attente…</div>
                                {/if}
                                <div class="cc-tags">
                                  {#each (l.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                                  {#each (l.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                                  {#if l.id_user}<span class="cc-tag cc-tag-author">👤 {l.id_user}</span>{/if}
                                </div>
                              </div>
                              <div class="cc-actions">
                                <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('liens', l.id, l.lien || '')}>ⓘ</button>
                                <button class="btn-test" on:click={async () => {
                                  // Re-fetch frais (URL temporaire qui expire) puis DL in-app
                                  try {
                                    const r = await GetDDLFilenameByLienID(l.id)
                                    if (r?.url) {
                                      ddlFilenames = { ...ddlFilenames, [l.id]: { state: 'ok', filename: r.filename || ddlFilenames[l.id]?.filename || '', url: r.url } }
                                      addLog('FICHE', `▶ DL #${l.id} → ~/Downloads`)
                                      const path = await DownloadToDownloads(r.url, r.filename || `lien-${l.id}`)
                                      addLog('FICHE', `✓ DL #${l.id} : ${path}`)
                                      try { Notify('✓ Téléchargement terminé', r.filename || `lien-${l.id}`) } catch(e) {}
                                    }
                                  } catch(e) { addLog('FICHE', `✗ DL lien #${l.id}: ${e}`) }
                                }} title="Re-debrid + télécharge dans ~/Downloads">⬇️ Télécharger</button>
                              </div>
                            </div>
                          {/each}
                        </div>
                      </details>
                    {/each}
                  {:else}
                  <div class="content-grid">
                    {#each fichesContent.liens.liens as l}
                      <div class="content-card">
                        <div class="cc-body">
                          <div class="cc-head">
                            <span class="cc-id">#{l.id}</span>
                            {#if l.qual?.qual}<span class="cc-chip cc-chip-qual">{l.qual.qual}</span>{/if}
                            {#if l.saison || l.episode}<span class="cc-chip cc-chip-se">S{String(l.saison||0).padStart(2,'0')}E{String(l.episode||0).padStart(2,'0')}</span>{/if}
                            {#if l.full_saison}<span class="cc-chip cc-chip-se">Saison complète</span>{/if}
                            {#if l.taille}<span class="cc-chip cc-chip-size">{fmtSize(l.taille)}</span>{/if}
                            {#if l.host?.name || l.id_host}<span class="cc-chip cc-chip-host">{l.host?.name || ('host#'+l.id_host)}</span>{/if}
                          </div>
                          {#if ddlFilenames[l.id]?.state === 'ok'}
                            {#if ddlFilenames[l.id].filename}
                              <div class="cc-name" title={ddlFilenames[l.id].filename}>{ddlFilenames[l.id].filename}</div>
                            {/if}
                            {#if ddlFilenames[l.id].url}
                              <div class="cc-name cc-name-mono cc-sub-url" title={ddlFilenames[l.id].url}>{ddlFilenames[l.id].url}</div>
                            {/if}
                          {:else if ddlFilenames[l.id]?.state === 'loading'}
                            <div class="cc-name-loading">⏳ Débridage + résolution du nom…</div>
                          {:else if ddlFilenames[l.id]?.state === 'err'}
                            <div class="cc-name-loading" style="color:#ff9585" title={ddlFilenames[l.id].error}>⚠ {ddlFilenames[l.id].error.length > 100 ? ddlFilenames[l.id].error.slice(0,100)+'…' : ddlFilenames[l.id].error}</div>
                          {:else}
                            <div class="cc-name-loading">⏳ En attente…</div>
                          {/if}
                          <div class="cc-tags">
                            {#each (l.langues_compact || []) as la}<span class="cc-tag cc-tag-lang">{langFlag(la.name)} {la.name}</span>{/each}
                            {#each (l.subs_compact || []) as s}<span class="cc-tag cc-tag-sub">💬 {s.name}</span>{/each}
                            {#if l.id_user}<span class="cc-tag cc-tag-author">👤 {l.id_user}</span>{/if}
                          </div>
                        </div>
                        <div class="cc-actions">
                          <button class="btn-test btn-icon" title="Voir le NFO" on:click={() => openNfo('liens', l.id, ddlFilenames[l.id]?.url || '')}>ⓘ</button>
                          <button class="btn-test" on:click={async () => {
                            try {
                              const r = await GetDDLFilenameByLienID(l.id)
                              if (r?.url) {
                                ddlFilenames = { ...ddlFilenames, [l.id]: { state: 'ok', filename: r.filename || ddlFilenames[l.id]?.filename || '', url: r.url } }
                                addLog('FICHE', `▶ DL #${l.id} → ~/Downloads`)
                                const path = await DownloadToDownloads(r.url, r.filename || `lien-${l.id}`)
                                addLog('FICHE', `✓ DL #${l.id} : ${path}`)
                                try { Notify('✓ Téléchargement terminé', r.filename || `lien-${l.id}`) } catch(e) {}
                              }
                            } catch(e) { addLog('FICHE', `✗ DL lien #${l.id}: ${e}`) }
                          }} title="Re-debrid + télécharge dans ~/Downloads">⬇️ Télécharger</button>
                        </div>
                      </div>
                    {/each}
                  </div>
                  {/if}
                {:else}
                  <div style="color:var(--text3);font-size:12px">Aucun DDL partagé via API pour cette fiche.</div>
                {/if}
              {/if}

              {#if fichesContent.charged > 0}
                <div style="margin-top:10px;color:var(--text3);font-size:11px">💰 Charged: {fichesContent.charged.toFixed(3)}€</div>
              {/if}
            {:else}
              <div style="color:#ff9585;font-size:12px">Erreur lors du chargement du contenu</div>
            {/if}
          </div>
        {/if}
      </div>

    <!-- ===== RESEED : Demandes team (sous-onglet) ===== -->
    {:else if activeTab === 'reseed' && reseedSubTab === 'requests'}
      <div class="tab-content">
        <h2>♻️ Reseed</h2>
        <div class="sub-tabs-nav">
          <button class:active={reseedSubTab === 'requests'} on:click={() => reseedSubTab = 'requests'}>📋 Demandes team</button>
          <button class:active={reseedSubTab === 'url'} on:click={() => reseedSubTab = 'url'}>🔗 Depuis URL</button>
        </div>
        <div class="section">
          <div class="section-header"><span>Filtres</span></div>
          <div class="field" style="display:flex;gap:6px;flex-wrap:wrap">
            <button class="btn-test" class:active-chip={reqFilter === 'pending'} on:click={() => { reqFilter = 'pending'; loadReseedRequests() }}>⏳ Pending</button>
            <button class="btn-test" class:active-chip={reqFilter === 'mine'} on:click={() => { reqFilter = 'mine'; loadReseedRequests() }}>👤 Les miennes</button>
            <button class="btn-test" class:active-chip={reqFilter === 'done'} on:click={() => { reqFilter = 'done'; loadReseedRequests() }}>✓ Done</button>
            <button class="btn-test" class:active-chip={reqFilter === 'rejected'} on:click={() => { reqFilter = 'rejected'; loadReseedRequests() }}>✗ Rejected</button>
            <button class="btn-test" class:active-chip={reqFilter === 'all'} on:click={() => { reqFilter = 'all'; loadReseedRequests() }}>📋 Toutes</button>
            <button class="btn-test" on:click={loadReseedRequests} disabled={reqLoading}>🔄 Rafraîchir</button>
            {#if reqFilter === 'pending' && reqList.length > 0}
              <button class="btn-save" on:click={processAllPending} disabled={reqLoading || Object.values(reqProcessing).some(v => v)}>
                ⚡ Tout traiter ({reqList.length})
              </button>
            {/if}
          </div>
        </div>

        <div class="section">
          <div class="section-header">
            <span>Demandes ({reqList.length})</span>
            {#if reqTotalPages > 1}
              <span style="color:var(--text3);font-size:11px">
                Page {reqPage}/{reqTotalPages}
                <button class="btn-test" style="margin-left:6px" on:click={() => { if (reqPage > 1) { reqPage--; loadReseedRequests() } }} disabled={reqPage <= 1}>‹</button>
                <button class="btn-test" on:click={() => { if (reqPage < reqTotalPages) { reqPage++; loadReseedRequests() } }} disabled={reqPage >= reqTotalPages}>›</button>
              </span>
            {/if}
          </div>

          {#if reqLoading && reqList.length === 0}
            <div style="color:var(--text3);font-size:12px">Chargement…</div>
          {:else if reqError}
            <div style="color:#ff9585;font-size:12px">⚠ {reqError}</div>
          {:else if reqList.length === 0}
            <div style="color:var(--text3);font-size:12px">Aucune demande dans ce filtre.</div>
          {:else}
            {#each reqList as req}
              {@const prog = reqProgress[req.id]}
              <div class="req-card">
                <div style="display:flex;gap:12px;align-items:flex-start">
                  {#if req.torrent?.title?.poster}
                    <img src={req.torrent.title.poster} alt="" style="width:56px;height:84px;object-fit:cover;border-radius:6px" loading="lazy" />
                  {/if}
                  <div style="flex:1;min-width:0">
                    <div style="font-weight:600;font-size:13px;color:var(--text);margin-bottom:4px">
                      {req.torrent?.title?.name || '(titre inconnu)'}
                      <span style="color:var(--text3);font-size:11px;font-weight:normal">· fiche #{req.torrent?.title_id}</span>
                    </div>
                    <div style="font-size:11px;color:var(--text3);word-break:break-all;margin-bottom:4px">{req.torrent?.torrent_name}</div>
                    <div style="display:flex;gap:12px;font-size:11px;color:var(--text3);flex-wrap:wrap">
                      <span>🌱 Seeders: <b style={req.torrent?.seeders === 0 ? 'color:#ff9585' : 'color:#7ef0c0'}>{req.torrent?.seeders ?? '?'}</b></span>
                      <span>👤 Demande: <b>{req.requester?.username || '#' + req.requester_id}</b></span>
                      <span>📤 Uploader: <b>{req.uploader?.username || '#' + req.uploader_id}</b></span>
                      <span>📅 {req.created_at?.slice(0,10)}</span>
                      <span class="req-status req-status-{req.status}">{req.status}</span>
                    </div>
                  </div>
                  <div style="display:flex;flex-direction:column;gap:6px;align-self:center">
                    <button class="btn-save" on:click={() => processReseedRequest(req)} disabled={reqProcessing[req.id] || req.status !== 'pending'} title="Reseed complet : DDL → FTP + .torrent → seedbox + force recheck">
                      {reqProcessing[req.id] ? '…' : '⚡ Traiter'}
                    </button>
                    <button class="btn-test" on:click={() => OpenBrowser(`https://hydracker.com/titles/${req.torrent?.title_id}`)}>🌐 Fiche</button>
                  </div>
                </div>
                {#if prog && (reqProcessing[req.id] || prog.stage === 'done' || prog.stage === 'error')}
                  <div class="req-progress">
                    <div style="display:flex;justify-content:space-between;font-size:11px;color:var(--text3);margin-bottom:4px">
                      <span style="color:{prog.stage === 'error' ? '#ff6b6b' : (prog.stage === 'done' ? '#7ef0c0' : 'var(--text2)')}">
                        {prog.stage === 'ftp' ? '⬆ FTP' : prog.stage === 'seedbox' ? '📦 Seedbox' : prog.stage === 'torrent_dl' ? '⬇ .torrent' : prog.stage === 'parsed' ? '✓ .torrent parsé' : prog.stage === 'ddl_search' ? '🔍 DDL' : prog.stage === 'ddl_picked' ? '🎯 DDL choisi' : prog.stage === 'token' ? '🔑 1fichier' : prog.stage === 'download' ? '⬇ DDL' : prog.stage === 'ftp_done' ? '✓ FTP' : prog.stage === 'recheck' ? '♻ Recheck' : prog.stage === 'done' ? '✓ Terminé' : prog.stage === 'error' ? '✗ Erreur' : prog.stage}
                        {#if prog.msg} — {prog.msg}{/if}
                      </span>
                      {#if prog.stage === 'ftp' && prog.total > 0}
                        <span style="font-family:monospace">
                          {prog.percent.toFixed(1)}% · {prog.speed_mb.toFixed(1)} MB/s · {(prog.bytes/1e9).toFixed(2)}/{(prog.total/1e9).toFixed(2)} GB
                        </span>
                      {/if}
                    </div>
                    {#if prog.stage !== 'error'}
                      <div class="progress-bar">
                        <div class="progress-fill" class:done={prog.stage === 'done'} style="width:{prog.percent || 0}%"></div>
                      </div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      </div>

    <!-- ===== MES UPLOADS ===== -->
    {:else if activeTab === 'myuploads'}
      <div class="tab-content">
        <h2>📤 Mes uploads</h2>
        <div class="section">
          <div class="section-header">
            <span>Type</span>
            <span style="color:var(--text3);font-size:11px">user: <b>{myUsername}</b></span>
          </div>
          <div class="field" style="display:flex;gap:6px;flex-wrap:wrap">
            <button class="btn-test" class:active-chip={myTab === 'liens'} on:click={() => { myTab = 'liens'; myPage = 1; loadMyUploads() }}>🔗 DDL</button>
            <button class="btn-test" class:active-chip={myTab === 'nzbs'} on:click={() => { myTab = 'nzbs'; myPage = 1; loadMyUploads() }}>📰 NZB</button>
            <button class="btn-test" on:click={loadMyUploads} disabled={myLoading}>🔄 Rafraîchir</button>
          </div>
        </div>

        <div class="section">
          <div class="section-header">
            <span>Items ({myItems.length})</span>
            {#if myTotalPages > 1}
              <span style="color:var(--text3);font-size:11px">
                Page {myPage}/{myTotalPages}
                <button class="btn-test" style="margin-left:6px" on:click={() => { if (myPage > 1) { myPage--; loadMyUploads() } }} disabled={myPage <= 1}>‹</button>
                <button class="btn-test" on:click={() => { if (myPage < myTotalPages) { myPage++; loadMyUploads() } }} disabled={myPage >= myTotalPages}>›</button>
              </span>
            {/if}
          </div>
          {#if myLoading && myItems.length === 0}
            <div style="color:var(--text3);font-size:12px">Chargement…</div>
          {:else if myError}
            <div style="color:#ff9585;font-size:12px">⚠ {myError}</div>
          {:else if myItems.length === 0}
            <div style="color:var(--text3);font-size:12px">Aucun item.</div>
          {:else}
            {#each myItems as item}
              <div class="my-item">
                <div style="flex:1;min-width:0">
                  <div style="font-weight:600;font-size:12px;color:var(--text);margin-bottom:3px">
                    #{item.id}
                    · {item.name || item.torrent_name || (myTab === 'liens' ? item.lien : '(sans nom)')}
                  </div>
                  <div style="display:flex;gap:10px;flex-wrap:wrap;font-size:11px;color:var(--text3)">
                    <span>fiche #{item.title_id}</span>
                    {#if item.qualite || item.quality}<span>qual: {qualityName(item.qualite || item.quality)}</span>{/if}
                    {#if item.saison || item.episode}<span>S{String(item.saison||0).padStart(2,'0')}E{String(item.episode||0).padStart(2,'0')}</span>{/if}
                    {#if item.taille || item.size}<span>{((item.taille||item.size)/1e9).toFixed(2)} GB</span>{/if}
                    {#if item.host?.name}<span>host: {item.host.name}</span>{/if}
                    {#if item.active === 0}<span style="color:#ff9585">⚠ inactif</span>{/if}
                    <span>📅 {(item.created_at || '').slice(0,10)}</span>
                  </div>
                </div>
                <div style="display:flex;flex-direction:column;gap:4px;align-self:center">
                  <button class="btn-test" on:click={() => startEdit(item)} title="Modifier">✏ Modifier</button>
                  <button class="btn-test" style="color:#ff6b6b;border-color:rgba(255,107,107,0.35)" on:click={() => startDelete(item)} title="Supprimer">🗑 Supprimer</button>
                </div>
              </div>
            {/each}
          {/if}
        </div>

        <!-- Modal Édition -->
        {#if editingItem}
          <div class="modal-backdrop" on:click|self={cancelEdit}>
            <div class="modal-card">
              <div class="modal-title">✏ Modifier #{editingItem.id}</div>
              <div class="modal-hint">Laisse à 0 les champs que tu ne veux pas changer.</div>
              <div class="field">
                <label>Qualité (ID)</label>
                <select bind:value={editPayload.quality}>
                  <option value={0}>— Inchangé —</option>
                  {#each qualityOptions as q}
                    <option value={q.id}>{q.name} (#{q.id})</option>
                  {/each}
                </select>
              </div>
              <div class="field" style="display:flex;gap:10px">
                <div style="flex:1">
                  <label>Saison</label>
                  <input type="number" min="0" bind:value={editPayload.saison} />
                </div>
                <div style="flex:1">
                  <label>Épisode</label>
                  <input type="number" min="0" bind:value={editPayload.episode} />
                </div>
              </div>
              <div class="field">
                <label>Langue (ID)</label>
                <input type="number" min="0" bind:value={editPayload.lang} placeholder="0 = inchangé · ex: 8 = TrueFrench" />
              </div>
              <div class="field">
                <label>État</label>
                <div style="display:flex;gap:6px">
                  <button class="btn-test" class:active-chip={editPayload.active === -1} on:click={() => editPayload.active = -1}>Inchangé</button>
                  <button class="btn-test" class:active-chip={editPayload.active === 1} on:click={() => editPayload.active = 1}>✓ Actif</button>
                  <button class="btn-test" class:active-chip={editPayload.active === 0} on:click={() => editPayload.active = 0}>⚠ Inactif</button>
                </div>
              </div>
              <div class="post-actions">
                <button class="btn-save" on:click={saveEdit}>💾 Enregistrer</button>
                <button class="btn-test" on:click={cancelEdit}>Annuler</button>
              </div>
            </div>
          </div>
        {/if}

        <!-- Modal Delete -->
        {#if deletingItem}
          <div class="modal-backdrop" on:click|self={cancelDelete}>
            <div class="modal-card" style="border-color:rgba(255,107,107,0.55)">
              <div class="modal-title" style="color:#ff6b6b">🗑 Suppression définitive</div>
              <div class="modal-hint">
                Tu es sur le point de supprimer <b>définitivement</b> cet item :
              </div>
              <div style="background:rgba(0,0,0,0.25);padding:10px;border-radius:8px;margin:10px 0;font-size:12px">
                <div><b>#{deletingItem.id}</b> · {deletingItem.name || deletingItem.torrent_name || deletingItem.lien || ''}</div>
                <div style="color:var(--text3);font-size:11px;margin-top:3px">fiche #{deletingItem.title_id}{deletingItem.saison || deletingItem.episode ? ` · S${String(deletingItem.saison||0).padStart(2,'0')}E${String(deletingItem.episode||0).padStart(2,'0')}` : ''}</div>
              </div>
              <div class="field">
                <label>Tape l'ID <b>{deletingItem.id}</b> pour confirmer :</label>
                <input type="text" bind:value={deleteConfirmInput} placeholder={deletingItem.id} />
              </div>
              <div class="post-actions">
                <button class="btn-save" style="background:#ff6b6b;border-color:#ff6b6b" on:click={confirmDelete} disabled={deleteConfirmInput.trim() !== String(deletingItem.id)}>
                  🗑 Supprimer définitivement
                </button>
                <button class="btn-test" on:click={cancelDelete}>Annuler</button>
              </div>
            </div>
          </div>
        {/if}
      </div>

    <!-- ===== LOGS : Requêtes API (sous-onglet) ===== -->
    {:else if activeTab === 'logs' && logsSubTab === 'api'}
      <div class="tab-content">
        <h2>🔬 Logs</h2>
        <div class="sub-tabs-nav">
          <button class:active={logsSubTab === 'journal'} on:click={() => logsSubTab = 'journal'}>📋 Journal app</button>
          <button class:active={logsSubTab === 'api'} on:click={() => logsSubTab = 'api'}>🔬 Requêtes API</button>
        </div>
        <div class="section">
          <div class="section-header">
            <span>Requêtes capturées ({filteredApiLogs.length}/{apiLogs.length})</span>
            <button class="btn-test" on:click={clearApiLogs}>🗑 Vider</button>
          </div>
          <div class="field" style="display:flex;gap:6px;flex-wrap:wrap;align-items:center">
            <button class="btn-test" class:active-chip={apiLogFilter === 'all'} on:click={() => apiLogFilter = 'all'}>Toutes</button>
            <button class="btn-test" class:active-chip={apiLogFilter === 'ok'} on:click={() => apiLogFilter = 'ok'}>✓ OK (2xx)</button>
            <button class="btn-test" class:active-chip={apiLogFilter === 'error'} on:click={() => apiLogFilter = 'error'}>✗ Erreurs</button>
            <input type="text" bind:value={apiLogSearch} placeholder="Filtrer par URL / body / erreur…" style="flex:1;min-width:200px" />
          </div>
        </div>

        <div class="section">
          {#if filteredApiLogs.length === 0}
            <div style="color:var(--text3);font-size:12px">Aucune requête (fais une action dans l'app pour en générer).</div>
          {:else}
            {#each filteredApiLogs as e, i}
              <div class="api-log-row" class:api-log-err={e.error || e.status >= 400} on:click={() => apiLogExpanded = { ...apiLogExpanded, [i]: !apiLogExpanded[i] }}>
                <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;font-size:11px">
                  <span style="color:var(--text3);font-family:monospace">{e.ts}</span>
                  <span class="api-log-method api-log-method-{e.method}">{e.method}</span>
                  <span style="color:{e.status >= 200 && e.status < 300 ? '#7ef0c0' : (e.status >= 400 ? '#ff6b6b' : 'var(--text3)')};font-weight:600;min-width:40px">{e.status || '—'}</span>
                  <span style="color:var(--text3);font-family:monospace">{e.duration_ms}ms</span>
                  <span style="flex:1;font-family:monospace;word-break:break-all;color:var(--text2)">{e.url}</span>
                </div>
                {#if e.error}
                  <div style="color:#ff6b6b;font-size:11px;margin-top:4px">⚠ {e.error}</div>
                {/if}
                {#if apiLogExpanded[i] && e.body_preview}
                  <pre style="background:rgba(0,0,0,0.3);padding:8px;border-radius:4px;margin-top:6px;font-size:10px;color:#7ef0c0;overflow-x:auto;white-space:pre-wrap;max-height:240px;overflow-y:auto">{e.body_preview}</pre>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      </div>

    <!-- ===== HISTORIQUE ===== -->
    {:else if activeTab === 'history'}
      <div class="tab-content">
        <h2>📚 Historique</h2>
        <div class="hist-stats">
          <span class="hist-stat">Total <b>{(histStats.nzb || 0) + (histStats.ddl || 0)}</b></span>
          <span class="hist-stat ok">OK <b>{histStats.ok}</b></span>
          <span class="hist-stat err">Erreurs <b>{histStats.error}</b></span>
          <span class="hist-stat">NZB <b>{histStats.nzb}</b></span>
          <span class="hist-stat">DDL <b>{histStats.ddl}</b></span>
        </div>
        <div class="hist-filters">
          <input class="hist-search" type="text" placeholder="🔍 Recherche (titre, fichier, lien…)" bind:value={histQuery} on:input={loadHistory} />
          <div class="hist-type-btns">
            <button class:active={histFilter === ''} on:click={() => { histFilter = ''; loadHistory() }}>Tous</button>
            <button class:active={histFilter === 'nzb'} on:click={() => { histFilter = 'nzb'; loadHistory() }}>📰 NZB</button>
            <button class:active={histFilter === 'ddl'} on:click={() => { histFilter = 'ddl'; loadHistory() }}>🔗 DDL</button>
          </div>
        </div>
        {#if histLoading}
          <div class="hist-empty">Chargement…</div>
        {:else if histEntries.length === 0}
          <div class="hist-empty">Aucune entrée. Les posts s'ajoutent automatiquement ici.</div>
        {:else}
          <div class="hist-list">
            {#each histEntries.filter(e => e.type !== 'torrent') as e}
              <div class="hist-row" class:err={e.status === 'error'}>
                <div class="hist-col-date">{formatDate(e.timestamp)}</div>
                <div class="hist-col-type hist-type-{e.type}">{e.type.toUpperCase()}</div>
                <div class="hist-col-main">
                  <div class="hist-title">{e.title_name || `#${e.title_id}`}{#if e.saison || e.episode} · S{String(e.saison).padStart(2,'0')}E{String(e.episode).padStart(2,'0')}{/if}</div>
                  <div class="hist-sub">{e.qualite_name || ''} · {e.filename}</div>
                  {#if e.links}<div class="hist-links">{e.links}</div>{/if}
                  {#if e.error}<div class="hist-error">✗ {e.error}</div>{/if}
                </div>
                <div class="hist-col-id">#{e.hydracker_id || '—'}</div>
                <button class="hist-del" on:click={() => deleteHistEntry(e.id)} title="Supprimer">✕</button>
              </div>
            {/each}
          </div>
        {/if}
      </div>

    <!-- ===== RESEED : Depuis URL (sous-onglet) ===== -->
    {:else if activeTab === 'reseed' && reseedSubTab === 'url'}
      <div class="tab-content">
        <h2>♻️ Reseed</h2>
        <div class="sub-tabs-nav">
          <button class:active={reseedSubTab === 'requests'} on:click={() => reseedSubTab = 'requests'}>📋 Demandes team</button>
          <button class:active={reseedSubTab === 'url'} on:click={() => reseedSubTab = 'url'}>🔗 Depuis URL</button>
        </div>

        <!-- Auto-reseed : workflow 1 clic depuis un ID / URL Hydracker.
             L'app liste les torrents dispos, choisit le meilleur (FR + qualité),
             télécharge le .torrent et l'envoie direct au ruTorrent. Aucun MKV
             ni FTP nécessaire — idéal pour traiter les demandes de reseed. -->
        <div class="section">
          <div class="section-header"><span>⚡ Auto-reseed depuis une fiche Hydracker</span></div>
          <div class="field">
            <label>URL ou ID fiche Hydracker</label>
            <input type="text" bind:value={autoReseedInput}
              placeholder="https://hydracker.com/titles/12345 ou 12345"
              on:keydown={e => e.key === 'Enter' && !autoReseedLoading && launchAutoReseed()} />
          </div>
          <div class="field" style="display:flex;gap:10px">
            <div style="flex:1">
              <label>Saison <span style="color:var(--text3);font-weight:normal;font-size:11px">(0 = film)</span></label>
              <input type="number" min="0" bind:value={autoReseedSaison} placeholder="0" />
            </div>
            <div style="flex:1">
              <label>Épisode <span style="color:var(--text3);font-weight:normal;font-size:11px">(0 = saison complète / film)</span></label>
              <input type="number" min="0" bind:value={autoReseedEpisode} placeholder="0" />
            </div>
          </div>
          <div class="post-actions" style="flex-wrap:wrap">
            <button class="btn-save" on:click={launchAutoReseed} disabled={autoReseedLoading || !autoReseedInput || !cfg.seedbox_url}
              title="Liste les torrents partagés via API → choisit le meilleur (FR + 1080p) → seedbox">
              {autoReseedLoading ? '…' : '⚡ Torrent → seedbox'}
            </button>
            <button class="btn-save" on:click={launchAutoReseedDDL} disabled={autoReseedLoading || !autoReseedInput || !cfg.ftp_host || !cfg.one_fichier_api_key}
              title="Aucun torrent partagé ? Télécharge depuis un DDL 1fichier et push direct sur le FTP configuré (streaming, sans passer par le disque)">
              {autoReseedLoading ? '…' : '📦 DDL → FTP'}
            </button>
            {#if !cfg.seedbox_url}
              <span style="color:var(--text3);font-size:11px;align-self:center">💡 Seedbox requis pour torrent</span>
            {/if}
            {#if !cfg.ftp_host || !cfg.one_fichier_api_key}
              <span style="color:var(--text3);font-size:11px;align-self:center">💡 FTP + clé API 1fichier requis pour DDL</span>
            {/if}
          </div>
          {#if autoReseedLoading && autoReseedDDLProgress.total > 0}
            <div class="recap-row" style="margin-top:8px">
              <span class="recap-key">Progression</span>
              <span class="recap-val">{autoReseedDDLProgress.percent.toFixed(1)}% · {autoReseedDDLProgress.speed_mb.toFixed(1)} MB/s · {(autoReseedDDLProgress.bytes/1e9).toFixed(2)} / {(autoReseedDDLProgress.total/1e9).toFixed(2)} GB</span>
            </div>
          {/if}
          {#if autoReseedLoading && autoReseedStatus}
            <div class="recap-row" style="margin-top:8px"><span class="recap-key">Statut</span><span class="recap-val">{autoReseedStatus}</span></div>
          {/if}
          {#if autoReseedError}
            <div class="recap-row" style="margin-top:8px"><span class="recap-key">Erreur</span><span class="recap-val" style="color:#ff6b6b">{autoReseedError}</span></div>
          {/if}
          {#if autoReseedResult}
            <div style="margin-top:12px;padding:10px;background:rgba(126,240,192,0.08);border:1px solid rgba(126,240,192,0.25);border-radius:8px">
              <div style="color:#7ef0c0;font-weight:600;margin-bottom:6px">✓ Torrent ajouté à la seedbox</div>
              <div class="recap-row"><span class="recap-key">Torrent</span><span class="recap-val">#{autoReseedResult.torrent_id} — {autoReseedResult.torrent_name}</span></div>
              {#if autoReseedResult.size_bytes}
                <div class="recap-row"><span class="recap-key">Taille</span><span class="recap-val">{(autoReseedResult.size_bytes/1e9).toFixed(2)} GB</span></div>
              {/if}
              {#if autoReseedResult.seedbox_path}
                <div class="recap-row"><span class="recap-key">Seedbox</span><span class="recap-val recap-file">{autoReseedResult.seedbox_path}</span></div>
              {/if}
            </div>
          {/if}
        </div>

        <div class="section">
          <div class="section-header"><span>Fichiers</span></div>
          <div class="field">
            <label>Fichier .torrent</label>
            <div class="pwd-row">
              <input type="text" value={reseedTorrentPath} readonly placeholder="Aucun fichier sélectionné" />
              <button class="btn-test" on:click={reseedPickTorrent}>Parcourir</button>
            </div>
          </div>
          <div class="field">
            <label>Fichier MKV <span style="color:var(--text3);font-weight:normal;font-size:11px">(optionnel — requis uniquement pour re-seed via seedbox)</span></label>
            <div class="pwd-row">
              <input type="text" value={reseedMkvPath} readonly placeholder="Optionnel si tu n'as pas de seedbox" />
              <button class="btn-test" on:click={reseedPickMkv}>Parcourir</button>
              {#if reseedMkvPath}<button class="btn-test" on:click={() => reseedMkvPath = ''} title="Retirer">✕</button>{/if}
            </div>
          </div>
          <div class="post-actions">
            <button class="btn-save" on:click={reseedAnalyze} disabled={!reseedTorrentPath || reseedPrepLoading}>
              {reseedPrepLoading ? '…' : '🔍 Analyser'}
            </button>
            <button class="btn-reset" on:click={reseedReset}>↺ Réinitialiser</button>
          </div>
        </div>

        {#if reseedPrep}
          <div class="section">
            <div class="section-header"><span>Résultat de l'analyse</span></div>
            <div class="recap-row"><span class="recap-key">Nom torrent</span><span class="recap-val">{reseedPrep.torrent_name}</span></div>
            <div class="recap-row"><span class="recap-key">Fichier principal</span><span class="recap-val">{reseedPrep.first_file_name}</span></div>
            <div class="recap-row"><span class="recap-key">Taille</span><span class="recap-val">{(reseedPrep.size / 1e9).toFixed(2)} GB</span></div>
            <div class="recap-row"><span class="recap-key">Info hash</span><span class="recap-val recap-file">{reseedPrep.info_hash}</span></div>
            {#if reseedPrep.search}
              <div class="recap-row"><span class="recap-key">TMDB</span><span class="recap-val recap-id">#{reseedPrep.search.tmdb_id} — {reseedPrep.search.title_fr || reseedPrep.search.title_vo} ({reseedPrep.search.year})</span></div>
            {:else}
              <div class="recap-row"><span class="recap-key">Recherche TMDB</span><span class="recap-val" style="color:#ff9585">Aucun match</span></div>
            {/if}
            {#if reseedPrep.hydracker_fiche}
              <div class="recap-row"><span class="recap-key">Fiche Hydracker</span><span class="recap-val" style="color:#7ef0c0">✓ {reseedPrep.hydracker_fiche.name} (#{reseedPrep.hydracker_fiche.id})</span></div>
              <div class="post-actions" style="gap:8px;flex-wrap:wrap">
                {#if reseedMkvPath && cfg.seedbox_url}
                  <button class="btn-save" on:click={reseedConfirm} disabled={reseedRunning} title="Upload MKV sur FTP + ajoute le .torrent à ruTorrent + force re-check">
                    {reseedRunning ? '…' : '🔄 Re-seed via seedbox'}
                  </button>
                {/if}
                <button class="btn-save" on:click={() => reseedOpenInHydracker(reseedPrep)} title="Ouvre l'onglet Hydracker avec le .torrent et la fiche pré-remplis">
                  🎬 Poster sur Hydracker
                </button>
                {#if !reseedMkvPath && cfg.seedbox_url}
                  <span style="color:var(--text3);font-size:11px;align-self:center">💡 Fournis un MKV pour activer le re-seed seedbox</span>
                {:else if !cfg.seedbox_url}
                  <span style="color:var(--text3);font-size:11px;align-self:center">💡 Configure une seedbox dans Réglages pour activer le re-seed</span>
                {/if}
                <span style="color:var(--text3);font-size:11px;align-self:center">💡 Pour lister/télécharger les sources, utilise l'onglet 🎞 Fiches</span>
              </div>
            {:else}
              <div class="recap-row"><span class="recap-key">Fiche Hydracker</span><span class="recap-val" style="color:#ff9585">✗ Pas de fiche correspondante</span></div>
            {/if}
          </div>
        {/if}

        {#if reseedStage}
          <div class="section">
            <div class="section-header"><span>Progression</span></div>
            <div class="nzb-live-status" class:done={reseedStage === 'done'}>{reseedMsg}</div>
            {#if reseedStage === 'ftp' || reseedStage === 'ftp_done'}
              <div class="nzb-live-step">
                <span>FTP</span>
                <div class="progress-bar"><div class="progress-fill" style="width:{reseedPct}%"></div></div>
                <span class="pct">{reseedPct.toFixed(0)}%</span>
              </div>
              <div class="nzb-live-meta">
                {#if reseedSpeed}<span>⚡ {reseedSpeed.toFixed(1)} MB/s</span>{/if}
              </div>
            {/if}
          </div>
        {/if}
      </div>

    <!-- ===== CHECK TORRENT ===== -->
    {:else if activeTab === 'check'}
      <div class="tab-content check-tab">
        <div class="check-header">
          <h2>🔍 Check Torrent</h2>
        </div>

        <!-- Section 1 : Mes torrents Hydracker (par seeders) -->
        <div class="section">
          {#if topSeedsView}
            <!-- Vue dédiée 3+ seeds (scan toutes les pages) -->
            <div class="section-header">
              <span style="display:flex;align-items:center;gap:10px">
                <button class="btn-test" on:click={() => { topSeedsView = false; topSeedsSelected = new Set() }}>← Retour</button>
                <span>🟢 Mes torrents 3+ seeds (toutes pages)</span>
                {#if topSeedsScanning}<span style="color:var(--text3);font-size:11px">{topSeedsScanProg}</span>{:else}<span style="color:var(--text3);font-size:11px">{topSeedsTorrents.length} torrents · {fmtBytes(topSeedsTorrents.reduce((a, t) => a + (t.taille || t.size || 0), 0))}</span>{/if}
              </span>
              <button class="btn-test" on:click={scanAllPagesFor3PlusSeeds} disabled={topSeedsScanning}>{topSeedsScanning ? '⏳ Scan…' : '🔄 Re-scan'}</button>
            </div>

            {#if topSeedsScanning && !topSeedsTorrents.length}
              <div style="text-align:center;padding:30px;color:var(--text3)">⏳ {topSeedsScanProg}</div>
            {:else if !topSeedsTorrents.length}
              <div style="text-align:center;padding:30px;color:var(--text3)">{topSeedsScanProg || 'Aucun torrent 3+ seeds trouvé.'}</div>
            {:else}
              <!-- Bulk delete bar -->
              {#if topSeedsSelected.size > 0}
                <div class="chk-bulk-bar" style="border-color:rgba(255,107,107,0.3);background:linear-gradient(90deg, rgba(255,107,107,0.08), rgba(255,107,107,0.02))">
                  <div>
                    <span class="chk-bulk-count" style="background:#ff6b6b;color:#fff">{topSeedsSelected.size}</span>
                    <span style="color:var(--text2)">torrent(s) sélectionné(s)</span>
                    <span style="color:var(--text3);margin-left:10px">· {fmtBytes(topSeedsSelectedSize)} à libérer</span>
                  </div>
                  <div style="display:flex;gap:8px;align-items:center">
                    {#if topSeedsBulkResult}<span style="color:#7ef0c0;font-size:12px">{topSeedsBulkResult}</span>{/if}
                    <button class="btn-test" on:click={() => topSeedsSelected = new Set()}>Désélectionner</button>
                    <button class="btn-bulk-delete" on:click={bulkDeleteTopSeeds} disabled={topSeedsBulkDeleting}>
                      {topSeedsBulkDeleting ? '⏳ Suppression…' : `🗑 Supprimer ${topSeedsSelected.size} torrent(s) + FTP`}
                    </button>
                  </div>
                </div>
              {/if}

              <div class="myseeds-list">
                <div class="myseed-header">
                  <label class="chk-cell-check">
                    <input type="checkbox" checked={topSeedsSelected.size === topSeedsTorrents.length && topSeedsTorrents.length > 0}
                      on:change={toggleTopSeedSelectAll} />
                    <span class="chk-checkbox-custom"></span>
                  </label>
                  <span>🌱 Seeds</span>
                  <span>ID</span>
                  <span>Nom</span>
                  <span style="text-align:right">Taille · S/E</span>
                  <span style="text-align:right">Actions</span>
                </div>
                {#each topSeedsTorrents as t (t.id)}
                  {@const seeders = t.seeders || 0}
                  {@const isSelected = topSeedsSelected.has(t.id)}
                  <div class="myseed-row myseed-row-selectable" class:selected={isSelected} style="border-left:3px solid #7ef0c0">
                    <label class="chk-cell-check" on:click|stopPropagation>
                      <input type="checkbox" checked={isSelected} on:change={() => toggleTopSeedSelect(t.id)} />
                      <span class="chk-checkbox-custom"></span>
                    </label>
                    <span class="myseed-seeds" style="color:#7ef0c0">{seeders}</span>
                    <span class="myseed-id">#{t.id}</span>
                    <span class="myseed-name" title={t.torrent_name || t.name || ''}>{t.torrent_name || t.name || '(sans nom)'}</span>
                    <span class="myseed-meta">
                      {#if t.taille || t.size}{((t.taille||t.size)/1e9).toFixed(1)}GB{/if}
                      {#if t.saison || t.episode} · S{String(t.saison||0).padStart(2,'0')}E{String(t.episode||0).padStart(2,'0')}{/if}
                    </span>
                    <div class="myseed-actions">
                      <button class="myseed-btn" on:click={() => OpenBrowser(`https://hydracker.com/titles/${t.title_id}`)} title="Voir la fiche">🌐</button>
                      <button class="myseed-btn danger" on:click={() => askDeleteTorrent(t)} disabled={mySeedsActioning[t.id]} title="Supprimer torrent + FTP">🗑</button>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          {:else}
          <div class="section-header">
            <span>🌱 Mes torrents Hydracker — par seeders ASC ({filteredMySeeds.length}/{mySeedsTorrents.length})</span>
            <div style="display:flex;gap:6px;align-items:center">
              {#if mySeedsTotalPages > 1}
                <span style="color:var(--text3);font-size:11px">Page {mySeedsPage}/{mySeedsTotalPages}</span>
                <button class="btn-test" on:click={() => { if (mySeedsPage > 1) { mySeedsPage--; loadMySeeds() } }} disabled={mySeedsPage <= 1}>‹</button>
                <button class="btn-test" on:click={() => { if (mySeedsPage < mySeedsTotalPages) { mySeedsPage++; loadMySeeds() } }} disabled={mySeedsPage >= mySeedsTotalPages}>›</button>
              {/if}
              <button class="btn-test" on:click={loadMySeeds} disabled={mySeedsLoading}>🔄 Rafraîchir</button>
            </div>
          </div>
          <div class="field" style="display:flex;gap:6px;flex-wrap:wrap">
            <button class="btn-test" class:active-chip={onlyMine} on:click={() => onlyMine = !onlyMine}
              title="Si actif : ne montre que les torrents que tu as ENCORE sur ta seedbox / qBit (filtre par info_hash)">
              {onlyMine ? '✅' : '⬜'} Sur ma seedbox uniquement{seedboxHashesLoaded ? ` (${seedboxHashes.size})` : ''}
            </button>
            <button class="btn-test" class:active-chip={mySeedsFilter === 'all'} on:click={() => mySeedsFilter = 'all'}>Tous ({mySeedsTorrents.length})</button>
            <button class="btn-test" class:active-chip={mySeedsFilter === '0seed'} on:click={() => mySeedsFilter = '0seed'} style="color:#ff6b6b">🔴 0 seed ({mySeedsTorrents.filter(t => (t.seeders||0) === 0).length})</button>
            <button class="btn-test" class:active-chip={mySeedsFilter === 'low'} on:click={() => mySeedsFilter = 'low'} style="color:#ffd60a">🟠 1-2 seeds ({mySeedsTorrents.filter(t => (t.seeders||0) > 0 && (t.seeders||0) <= 2).length})</button>
            <button class="btn-test" style="color:#7ef0c0;border-color:rgba(126,240,192,0.4)" on:click={() => { topSeedsView = true; if (!topSeedsTorrents.length) scanAllPagesFor3PlusSeeds() }}
              title="Scanne toutes les pages et liste tous les torrents avec 3+ seeds (vue dédiée + bulk delete)">
              🟢 3+ seeds (toutes pages) →
            </button>
            <button class="btn-test" on:click={checkLocalMkv} disabled={localMkvChecking} title="Vérifier qu'un MKV local correspond à une fiche Hydracker (et voir les sources DDL/torrents dispos)">
              {localMkvChecking ? '…' : '🔍 Parcourir un MKV local'}
            </button>
          </div>

          {#if mySeedsLoading && !mySeedsTorrents.length}
            <div style="color:var(--text3);font-size:12px;margin-top:10px">Chargement…</div>
          {:else if mySeedsError}
            <div style="color:#ff6b6b;font-size:12px;margin-top:10px">⚠ {mySeedsError}</div>
          {:else if filteredMySeeds.length === 0}
            <div style="color:var(--text3);font-size:12px;margin-top:10px">Aucun torrent dans ce filtre.</div>
          {:else}
            <div class="myseeds-list">
              {#each filteredMySeeds as t}
                {@const seeders = t.seeders || 0}
                {@const seedColor = seeders === 0 ? '#ff6b6b' : (seeders <= 2 ? '#ffd60a' : '#7ef0c0')}
                <div class="myseed-row" style="border-left:3px solid {seedColor}">
                  <span class="myseed-seeds" style="color:{seedColor}">{seeders}</span>
                  <span class="myseed-id">#{t.id}</span>
                  <span class="myseed-name" title={t.torrent_name || t.name || ''}>{t.torrent_name || t.name || '(sans nom)'}</span>
                  <span class="myseed-meta">
                    {#if t.taille || t.size}{((t.taille||t.size)/1e9).toFixed(1)}GB{/if}
                    {#if t.saison || t.episode} · S{String(t.saison||0).padStart(2,'0')}E{String(t.episode||0).padStart(2,'0')}{/if}
                  </span>
                  <div class="myseed-actions">
                    <button class="myseed-btn" on:click={() => autoReseedFromCheck(t)} disabled={mySeedsActioning[t.id]} title="Push .torrent sur seedbox">⚡</button>
                    {#if cfg.ftp_host && cfg.one_fichier_api_key}
                      <button class="myseed-btn primary" on:click={() => fullReseedFromCheck(t)} disabled={mySeedsActioning[t.id]} title="Reseed complet : DDL → FTP + seedbox">⚡⚡</button>
                    {/if}
                    <button class="myseed-btn" on:click={() => OpenBrowser(`https://hydracker.com/titles/${t.title_id}`)} title="Voir la fiche sur Hydracker">🌐</button>
                    <button class="myseed-btn danger" on:click={() => askDeleteTorrent(t)} disabled={mySeedsActioning[t.id]} title="Supprimer le torrent + FTP">🗑</button>
                  </div>
                </div>
                {#if checkActionProg[t.id]}
                  {@const prog = checkActionProg[t.id]}
                  <div class="check-prog" class:done={prog.stage === 'done'} class:err={prog.stage === 'error'}>
                    <div class="check-prog-bar"><div class="check-prog-fill" style="width:{prog.percent || 0}%"></div></div>
                    <div class="check-prog-meta">
                      <span class="check-prog-msg">{prog.msg || prog.stage || '…'}</span>
                      <span class="check-prog-stats">{(prog.percent || 0).toFixed(0)}%{#if prog.speed_mb > 0} · ⚡ {prog.speed_mb.toFixed(1)} MB/s{/if}</span>
                    </div>
                  </div>
                {/if}
              {/each}
            </div>
          {/if}
          {/if}
        </div>

        <!-- Modal confirmation suppression torrent + FTP -->
        {#if deleteTorrentModal}
          <div class="modal-backdrop" on:click|self={closeDeleteTorrentModal}>
            <div class="modal-card" style="max-width:560px">
              <div class="modal-title" style="color:#ff6b6b">🗑 Suppression définitive</div>
              {#if !deleteTorrentModal.result}
                <div class="modal-hint">{deleteTorrentModal.torrent.torrent_name || deleteTorrentModal.torrent.name || '#' + deleteTorrentModal.torrent.id}</div>
                <div style="margin:14px 0;font-size:12px;line-height:1.5;color:var(--text2)">
                  Cette action va :<br>
                  1️⃣ Supprimer le(s) <b>.mkv</b> sur le FTP (perso puis mod en fallback)<br>
                  2️⃣ Supprimer le <b>.torrent</b> de ta seedbox (rutorrent / qBit)<br>
                  <br>
                  <span style="color:#7ef0c0">✓ Le post Hydracker reste intact</span> (fiche communautaire préservée).<br>
                  <span style="color:#ff9585">⚠ Irréversible côté seedbox/FTP.</span>
                </div>
                {#if deleteTorrentModal.error}
                  <div style="color:#ff6b6b;font-size:12px;margin:10px 0">⚠ {deleteTorrentModal.error}</div>
                {/if}
                <div style="display:flex;gap:8px;justify-content:flex-end;align-items:center;margin-top:14px">
                  <button class="btn-test" style="margin:0;padding:10px 18px;font-size:13px;font-weight:600;letter-spacing:0.3px;text-transform:none;line-height:1.2" on:click={closeDeleteTorrentModal} disabled={deleteTorrentModal.loading}>Annuler</button>
                  <button class="btn-save" style="margin:0;background:#7a1c1c;color:#fff;padding:10px 18px;font-size:13px;font-weight:600;letter-spacing:0.3px;line-height:1.2" on:click={confirmDeleteTorrent} disabled={deleteTorrentModal.loading}>
                    {deleteTorrentModal.loading ? '⏳ Suppression…' : '🗑 Confirmer la suppression'}
                  </button>
                </div>
              {:else}
                {@const r = deleteTorrentModal.result}
                <div style="margin:14px 0;font-size:12px;line-height:1.6">
                  <div style="color:{r.hydracker_ok ? '#7ef0c0' : '#ff6b6b'}">
                    {r.hydracker_ok ? '✓' : '✗'} Hydracker : {r.hydracker_ok ? 'torrent supprimé' : (r.hydracker_err || 'échec')}
                  </div>
                  <div style="color:{r.seedbox_ok ? '#7ef0c0' : '#ffd60a'};margin-top:6px">
                    {r.seedbox_ok ? '✓' : '⚠'} Seedbox : {r.seedbox_ok ? `torrent retiré (${r.used_seedbox})` : (r.seedbox_err || 'pas de seedbox configurée')}
                  </div>
                  <div style="color:{r.ftp_deleted?.length ? '#7ef0c0' : '#ffd60a'};margin-top:6px">
                    {r.ftp_deleted?.length ? '✓' : '⚠'} FTP : {r.ftp_deleted?.length || 0} fichier(s) supprimé(s)
                    {#if r.used_ftp}<span style="color:var(--text3)"> (via {r.used_ftp})</span>{/if}
                  </div>
                  {#if r.ftp_deleted?.length}
                    <ul style="font-size:11px;color:var(--text3);margin:6px 0 0 22px">
                      {#each r.ftp_deleted as f}<li>{f}</li>{/each}
                    </ul>
                  {/if}
                  {#if r.ftp_errors?.length && !r.ftp_deleted?.length}
                    <div style="font-size:11px;color:#ff9585;margin-top:6px">Erreurs FTP :</div>
                    <ul style="font-size:11px;color:#ff9585;margin:4px 0 0 22px">
                      {#each r.ftp_errors as e}<li>{e}</li>{/each}
                    </ul>
                  {/if}
                </div>
                <div style="display:flex;justify-content:flex-end;margin-top:12px">
                  <button class="btn-save" on:click={closeDeleteTorrentModal}>Fermer</button>
                </div>
              {/if}
            </div>
          </div>
        {/if}

        <!-- Modal résultat du check MKV local -->
        {#if localMkvCheck}
          <div class="modal-backdrop" on:click|self={closeLocalMkvCheck}>
            <div class="modal-card" style="max-width:600px">
              <div class="modal-title">🔍 Vérification MKV local</div>
              <div class="modal-hint">{localMkvCheck.filename}</div>
              {#if localMkvCheck.error}
                <div style="color:#ff6b6b;font-size:12px;margin:10px 0">⚠ {localMkvCheck.error}</div>
              {:else if localMkvCheck.parsed}
                <div style="display:grid;grid-template-columns:max-content 1fr;gap:6px 12px;margin:10px 0;font-size:12px">
                  <span style="color:var(--text3)">Titre détecté</span><span>{localMkvCheck.parsed.title || '—'}</span>
                  {#if localMkvCheck.parsed.year}<span style="color:var(--text3)">Année</span><span>{localMkvCheck.parsed.year}</span>{/if}
                  {#if localMkvCheck.parsed.season || localMkvCheck.parsed.episode}<span style="color:var(--text3)">S/E</span><span>S{String(localMkvCheck.parsed.season||0).padStart(2,'0')}E{String(localMkvCheck.parsed.episode||0).padStart(2,'0')}</span>{/if}
                </div>
                {#if localMkvCheck.hydrackerFiche}
                  <div style="background:rgba(0,180,216,0.05);border:1px solid rgba(0,180,216,0.25);border-radius:8px;padding:10px;margin:8px 0">
                    <div style="font-weight:600;font-size:13px">✓ Fiche : {localMkvCheck.hydrackerFiche.name} (#{localMkvCheck.hydrackerFiche.id})</div>
                    {#if localMkvCheck.content}
                      <div style="display:flex;gap:14px;margin-top:8px;font-size:12px">
                        <span>📦 Torrents : <b>{localMkvCheck.content.torrents?.torrents?.length || 0}</b></span>
                        <span>📰 NZB : <b>{localMkvCheck.content.nzbs?.nzbs?.length || 0}</b></span>
                        <span>🔗 DDL : <b>{localMkvCheck.content.liens?.liens?.length || 0}</b></span>
                      </div>
                      {#if localMkvCheck.content.liens?.liens?.length}
                        <div style="color:#7ef0c0;font-size:12px;margin-top:8px">✓ DDL dispo — auto-reseed DDL→FTP possible</div>
                      {/if}
                    {/if}
                  </div>
                  <div class="post-actions" style="flex-wrap:wrap;gap:6px">
                    {#if localMkvCheck.content?.liens?.liens?.length && cfg.ftp_host && cfg.one_fichier_api_key}
                      <button class="btn-save" style="background:#7ef0c0;color:#000"
                        on:click={async () => {
                          const fid = localMkvCheck.hydrackerFiche.id
                          const sa = localMkvCheck.parsed?.season || 0
                          const ep = localMkvCheck.parsed?.episode || 0
                          closeLocalMkvCheck()
                          autoReseedInput = String(fid)
                          autoReseedSaison = sa
                          autoReseedEpisode = ep
                          activeTab = 'reseed'
                          setTimeout(() => launchAutoReseedDDL(), 300)
                        }}
                        title="Téléchargement DDL 1fichier → streaming direct vers FTP configuré">
                        ⚡ Lancer DDL → FTP
                      </button>
                    {:else if localMkvCheck.content?.liens?.liens?.length}
                      <span style="color:var(--text3);font-size:11px;align-self:center">💡 Configure FTP + clé API 1fichier pour activer DDL→FTP</span>
                    {/if}
                    {#if localMkvCheck.content?.torrents?.torrents?.length && cfg.seedbox_url}
                      <button class="btn-save"
                        on:click={async () => {
                          const fid = localMkvCheck.hydrackerFiche.id
                          const sa = localMkvCheck.parsed?.season || 0
                          const ep = localMkvCheck.parsed?.episode || 0
                          closeLocalMkvCheck()
                          autoReseedInput = String(fid)
                          autoReseedSaison = sa
                          autoReseedEpisode = ep
                          activeTab = 'reseed'
                          setTimeout(() => launchAutoReseed(), 300)
                        }}
                        title="Auto-reseed via torrent Hydracker → seedbox">
                        ⚡ Torrent → seedbox
                      </button>
                    {/if}
                    <button class="btn-save" on:click={() => { activeTab = 'fiches'; fichesMode = 'hydracker_id'; fichesQuery = String(localMkvCheck.hydrackerFiche.id); fichesSearch(); closeLocalMkvCheck() }}>
                      🎞 Ouvrir dans Fiches
                    </button>
                    <button class="btn-test" on:click={closeLocalMkvCheck}>Fermer</button>
                  </div>
                {/if}
              {/if}
              {#if localMkvChecking}
                <div style="color:var(--text3);font-size:12px;margin-top:10px">Recherche…</div>
              {/if}
            </div>
          </div>
        {/if}

        <!-- Section 2 : Seedbox locale — vue compacte avec sélection multiple -->
        <div class="section" class:section-collapsed={!seedboxSectionOpen}>
          <div class="section-header section-header-toggle" style="margin-bottom:{seedboxSectionOpen ? 14 : 0}px"
            on:click={() => seedboxSectionOpen = !seedboxSectionOpen}>
            <span style="display:flex;align-items:center;gap:8px">
              <span class="section-chevron" class:open={seedboxSectionOpen}>▶</span>
              📂 Ma seedbox
              {#if checkTorrents.length > 0}
                <span style="color:var(--text3);font-size:11px;font-weight:400">({checkTorrents.length} torrents)</span>
              {/if}
            </span>
            <button class="btn-test" on:click|stopPropagation={() => { clearCheckSelection(); seedboxSectionOpen = true; loadCheckTorrents(false) }} disabled={checkLoading}>
              {checkLoading ? '⏳ Chargement…' : '🔄 Charger / Rafraîchir'}
            </button>
          </div>

          {#if seedboxSectionOpen}
          {#if !checkTorrents.length && !checkLoading}
            <p class="coming-soon">Clique sur <b>Charger</b> pour lister les torrents de la seedbox.</p>
          {:else if checkTorrents.length > 0}
            <!-- Toolbar : filtres + sélection bulk + tri -->
            <div class="chk-toolbar">
              <div class="chk-toolbar-left">
                <button class="filter-btn" class:active={checkFilter === 'all'} on:click={() => checkFilter = 'all'}>
                  Tous <span class="filter-count">{checkTorrents.length}</span>
                </button>
                <button class="filter-btn" class:active={checkFilter === 'active'} on:click={() => checkFilter = 'active'}>
                  ✓ Actifs <span class="filter-count">{checkTorrents.filter(t => t.is_active === 1).length}</span>
                </button>
                <button class="filter-btn" class:active={checkFilter === 'inactive'} on:click={() => checkFilter = 'inactive'}>
                  ⚠ Inactifs <span class="filter-count">{checkTorrents.filter(t => isIncomplete(t)).length}</span>
                </button>
              </div>
              <div class="chk-toolbar-right">
                <label class="chk-sort-label">
                  <span style="color:var(--text3);font-size:11px;text-transform:uppercase;letter-spacing:0.5px">Trier</span>
                  <select bind:value={checkSort} class="chk-sort-select">
                    <option value="seeds_desc">🌱 Seeds (+ → -)</option>
                    <option value="seeds_asc">🌱 Seeds (- → +)</option>
                    <option value="size_desc">📦 Taille (+ → -)</option>
                    <option value="name">🔤 Nom A-Z</option>
                  </select>
                </label>
              </div>
            </div>

            <!-- Bulk action bar (visible si sélection) -->
            {#if checkSelected.size > 0}
              <div class="chk-bulk-bar">
                <div>
                  <span class="chk-bulk-count">{checkSelected.size}</span>
                  <span style="color:var(--text2)">torrent(s) sélectionné(s)</span>
                  <span style="color:var(--text3);margin-left:10px">· {fmtBytes(checkSelectedSize)} à libérer</span>
                </div>
                <div style="display:flex;gap:8px;align-items:center">
                  {#if checkDeleteResult}
                    <span style="color:#7ef0c0;font-size:12px">{checkDeleteResult}</span>
                  {/if}
                  <button class="btn-test" on:click={clearCheckSelection}>Désélectionner</button>
                  <button class="btn-bulk-delete" on:click={bulkDeleteSelected} disabled={checkDeleting}>
                    {checkDeleting ? '⏳ Suppression…' : `🗑 Supprimer ${checkSelected.size} torrent(s) + FTP`}
                  </button>
                </div>
              </div>
            {/if}

            <!-- Table compacte -->
            <div class="chk-table">
              <div class="chk-thead">
                <label class="chk-cell-check">
                  <input type="checkbox" checked={pagedAllSelected} on:change={toggleCheckSelectAllPaged} />
                  <span class="chk-checkbox-custom"></span>
                </label>
                <div class="chk-cell-seeds">🌱 Seeds</div>
                <div class="chk-cell-size">📦 Taille</div>
                <div class="chk-cell-name">Nom</div>
                <div class="chk-cell-status">Statut</div>
                <div class="chk-cell-actions"></div>
              </div>
              {#each pagedCheckTorrents as t (t.hash)}
                {@const seeds = seedsOf(t)}
                {@const st = checkState[t.hash] || {}}
                {@const isSelected = checkSelected.has(t.hash)}
                {@const isExpanded = checkExpanded === t.hash}
                <div class="chk-row" class:selected={isSelected} class:expanded={isExpanded}>
                  <div class="chk-row-main" on:click={() => checkExpanded = isExpanded ? '' : t.hash}>
                    <label class="chk-cell-check" on:click|stopPropagation>
                      <input type="checkbox" checked={isSelected} on:change={() => toggleCheckSelect(t.hash)} />
                      <span class="chk-checkbox-custom"></span>
                    </label>
                    <div class="chk-cell-seeds">
                      <span class="chk-seeds-chip" class:zero={seeds === 0} class:low={seeds > 0 && seeds <= 2} class:mid={seeds > 2 && seeds <= 10} class:high={seeds > 10}>
                        {seeds}
                      </span>
                    </div>
                    <div class="chk-cell-size">{fmtBytes(t.size)}</div>
                    <div class="chk-cell-name" title={t.name}>{t.name}</div>
                    <div class="chk-cell-status">
                      {#if t.has_error}
                        <span class="chk-badge err">⚠</span>
                      {:else if t.is_active}
                        <span class="chk-badge ok">✓</span>
                      {:else}
                        <span class="chk-badge">⏸</span>
                      {/if}
                    </div>
                    <div class="chk-cell-actions" on:click|stopPropagation>
                      <button class="mgr-icon-btn mgr-icon-danger" title="Supprimer de la seedbox" on:click={() => deleteOne(t.hash)}>🗑</button>
                      <span class="chk-chevron" class:open={isExpanded}>›</span>
                    </div>
                  </div>
                  {#if isExpanded}
                    <div class="chk-row-detail">
                      {#if t.file_name}
                        <div class="chk-detail-row"><span class="chk-detail-label">📄 Fichier</span><span class="chk-detail-val">{t.file_name}</span></div>
                      {/if}
                      <div class="chk-detail-row"><span class="chk-detail-label">🔑 Hash</span><span class="chk-detail-val mono">{t.hash}</span></div>
                      {#if t.message}
                        <div class="chk-detail-row"><span class="chk-detail-label">💬 Message</span><span class="chk-detail-val">{t.message}</span></div>
                      {/if}
                      {#if t.lihdl_url}
                        <div class="chk-detail-row">
                          <span class="chk-detail-label">✓ Match LiHDL</span>
                          <span class="chk-detail-val mono">{t.lihdl_name}</span>
                        </div>
                        {#if st.stage && st.stage !== 'done'}
                          <div class="chk-progress">
                            <div class="chk-status">{st.msg}{#if st.speed} · {st.speed.toFixed(1)} MB/s{/if}</div>
                            {#if st.percent >= 0}
                              <div class="progress-bar"><div class="progress-fill" style="width:{st.percent}%"></div></div>
                              <span class="chk-pct">{(st.percent || 0).toFixed(0)}%</span>
                            {/if}
                          </div>
                        {:else if st.stage === 'done'}
                          <div class="chk-done">✓ {st.msg}</div>
                        {:else}
                          <button class="btn-save chk-btn" on:click={() => reseedOne(t)}>⬇ Télécharger et re-seed</button>
                        {/if}
                      {:else}
                        <div class="chk-detail-row">
                          <span class="chk-detail-label">Match LiHDL</span>
                          <div style="display:flex;gap:6px;flex-wrap:wrap;align-items:center;flex:1">
                            <span style="color:var(--text3);font-size:12px">— pas de MKV correspondant sur LiHDL</span>
                            <button class="btn-test" on:click={() => browseLocalForRow(t)}>📁 Parcourir mon Mac</button>
                            <button class="btn-test" on:click={() => searchFreeLink(t)} disabled={seedboxLocalSearch[t.hash]?.state === 'searching'}>
                              {seedboxLocalSearch[t.hash]?.state === 'searching' ? '⏳ Recherche…' : '🔍 Chercher un DDL gratuit'}
                            </button>
                          </div>
                        </div>
                        {#if seedboxLocalSearch[t.hash]?.state === 'ok'}
                          <div class="chk-detail-row">
                            <span class="chk-detail-label">Résultats</span>
                            <div style="flex:1;display:flex;flex-direction:column;gap:6px">
                              {#each seedboxLocalSearch[t.hash].results.slice(0, 5) as r}
                                <div style="display:flex;gap:8px;align-items:center;padding:6px 10px;background:rgba(126,240,192,0.05);border:1px solid rgba(126,240,192,0.2);border-radius:8px;font-size:12px">
                                  <span style="flex:1;color:var(--text);overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title={r.name || r.title || r.url}>{r.name || r.title || r.url}</span>
                                  {#if r.url}
                                    <button class="btn-test" on:click={() => OpenBrowser(r.url)}>🌐 Ouvrir</button>
                                  {/if}
                                </div>
                              {/each}
                            </div>
                          </div>
                        {:else if seedboxLocalSearch[t.hash]?.state === 'nomatch'}
                          <div class="chk-detail-row"><span class="chk-detail-label"></span><span style="color:var(--text3);font-size:12px">Aucun DDL gratuit trouvé pour ce nom</span></div>
                        {:else if seedboxLocalSearch[t.hash]?.state === 'err'}
                          <div class="chk-detail-row"><span class="chk-detail-label"></span><span style="color:#ff9585;font-size:12px">⚠ {seedboxLocalSearch[t.hash].error}</span></div>
                        {/if}
                      {/if}
                    </div>
                  {/if}
                </div>
              {/each}
            </div>

            <!-- Pagination en bas -->
            {#if filteredCheckTorrents.length > checkPageSize}
              <div class="check-pagination">
                <button class="btn-test" on:click={() => checkPage = Math.max(1, checkPage - 1)} disabled={checkPage <= 1}>← Préc</button>
                <span style="color:var(--text2);font-size:12px">
                  Page {checkPage} / {checkTotalPages} — {filteredCheckTorrents.length} torrents · {(checkPage - 1) * checkPageSize + 1}–{Math.min(checkPage * checkPageSize, filteredCheckTorrents.length)}
                </span>
                <button class="btn-test" on:click={() => checkPage = Math.min(checkTotalPages, checkPage + 1)} disabled={checkPage >= checkTotalPages}>Suiv →</button>
              </div>
            {/if}
          {/if}
          {/if}
        </div>
      </div>

    <!-- ===== MANAGER (admin only — users + rôles & accès) ===== -->
    {:else if activeTab === 'manager' && myRole === 'admin'}
      <div class="tab-content">
        <h2 style="display:flex;align-items:center;gap:12px">
          👥 Manager
          <span style="font-size:12px;color:var(--text3);font-weight:500">·  gestion des utilisateurs et permissions</span>
        </h2>

        <div class="manager-tabs">
          <button class:active={managerView === 'users'} on:click={() => managerView = 'users'}>
            👤 Utilisateurs <span class="count">{teamUsers.length}</span>
          </button>
          <button class:active={managerView === 'roles'} on:click={() => managerView = 'roles'}>
            🎭 Rôles & accès <span class="count">{Object.keys(teamRoles).length}</span>
          </button>
        </div>

        {#if managerLoading}
          <div style="text-align:center;padding:40px;color:var(--text3)">⏳ Chargement de team.json…</div>
        {:else if managerError}
          <div class="mgr-alert">⚠ {managerError} <button class="mgr-alert-close" on:click={() => managerError = ''}>✕</button></div>
        {/if}

        {#if !managerLoading && managerView === 'users'}
          <div class="mgr-users-grid">
            {#each teamUsers as u (u.pseudo)}
              {@const r = teamRoles[u.role] || {}}
              <div class="mgr-user-card">
                <div class="mgr-user-avatar" style="background:linear-gradient(135deg, {r.color || '#60a5fa'}55, {r.color || '#60a5fa'}22);color:{r.color || '#60a5fa'}">
                  {u.pseudo.charAt(0).toUpperCase()}
                </div>
                <div class="mgr-user-info">
                  <div class="mgr-user-name">{u.pseudo} {#if u.pseudo === myUsername}<span class="mgr-you">toi</span>{/if}</div>
                  <div class="mgr-user-sub">
                    <span style="color:{r.color || '#60a5fa'}">{r.badge || '🔵'} {u.title || r.title || u.role}</span>
                  </div>
                </div>
                <div class="mgr-user-actions">
                  <button class="mgr-icon-btn" title="Modifier" on:click={() => { editUserTarget = { ...u, newPassword: '' }; editUserOpen = true }}>✏️</button>
                  <button class="mgr-icon-btn mgr-icon-danger" title="Supprimer" on:click={() => manageDeleteUser(u.pseudo)} disabled={u.pseudo === myUsername}>🗑</button>
                </div>
                {#if managerNewPasswords[u.pseudo]}
                  <div class="mgr-user-badge">🔄 nouveau mdp</div>
                {/if}
              </div>
            {/each}
            <button class="mgr-add-card" on:click={() => { newUserForm = { pseudo: '', password: '', role: Object.keys(teamRoles)[0] || 'user', title: '' }; addUserOpen = true }}>
              <div style="font-size:28px;line-height:1">➕</div>
              <div>Ajouter un utilisateur</div>
            </button>
          </div>
        {/if}

        {#if !managerLoading && managerView === 'roles'}
          <div class="mgr-role-chips">
            {#each Object.entries(teamRoles) as [key, r] (key)}
              <button class="mgr-role-chip" class:active={selectedRole === key}
                style="border-color:{selectedRole === key ? r.color : 'transparent'};
                       background: {selectedRole === key ? r.color + '20' : 'rgba(255,255,255,0.04)'};
                       color: {r.color}"
                on:click={() => selectedRole = key}>
                <span style="font-size:16px">{r.badge}</span>
                <span style="font-weight:600">{key}</span>
                <span style="color:var(--text3);font-size:11px">· {(r.tabs||[]).length}/{TABS.length}</span>
                {#if key !== 'admin'}
                  <span class="mgr-chip-del" on:click|stopPropagation={() => manageDeleteRole(key)} title="Supprimer ce rôle">×</span>
                {/if}
              </button>
            {/each}
            <button class="mgr-role-chip mgr-add-chip" on:click={() => { newRoleForm = { slug: '', badge: '🟣', color: '#a78bfa', title: '' }; addRoleOpen = true }}>
              ➕ Créer un rôle
            </button>
          </div>

          {#if selectedRole && teamRoles[selectedRole]}
            {@const r = teamRoles[selectedRole]}
            <div class="mgr-role-detail">
              <div class="mgr-role-header">
                <div style="display:flex;gap:14px;align-items:center">
                  <div class="mgr-role-emoji" style="background:{r.color}15;color:{r.color}">{r.badge}</div>
                  <div>
                    <div style="font-size:18px;font-weight:700;color:{r.color}">{selectedRole}</div>
                    <div style="font-size:12px;color:var(--text3)">
                      {(r.tabs||[]).length} onglet(s) visible(s) sur {TABS.length}
                    </div>
                  </div>
                </div>
                <div style="display:flex;gap:8px;align-items:center">
                  <input type="text" value={r.title || ''} on:input={(e) => manageUpdateRoleMeta(selectedRole, 'title', e.target.value)}
                    placeholder="Titre par défaut" style="font-size:12px;padding:6px 10px" />
                  <input type="color" value={r.color} on:input={(e) => manageUpdateRoleMeta(selectedRole, 'color', e.target.value)}
                    style="width:36px;height:36px;padding:0;border-radius:8px;cursor:pointer;border:1px solid var(--border)" title="Couleur" />
                </div>
              </div>

              <div class="mgr-role-tabs">
                {#each TABS as t}
                  {@const enabled = (r.tabs || []).includes(t.id)}
                  <label class="mgr-toggle-row">
                    <span class="mgr-toggle-label">{t.label}</span>
                    <input type="checkbox" checked={enabled}
                      on:change={() => manageToggleTab(selectedRole, t.id)} />
                    <span class="mgr-slider" style="--active-color: {r.color}"></span>
                  </label>
                {/each}
              </div>

              <!-- Permissions custom (boutons/actions spécifiques) -->
              <div style="margin-top:14px;padding-top:14px;border-top:1px solid var(--border)">
                <div style="font-size:11px;color:var(--text3);margin-bottom:8px;text-transform:uppercase;letter-spacing:0.5px">🔓 Permissions spéciales</div>
                <div class="mgr-role-tabs">
                  {#each ROLE_PERMISSIONS as p}
                    {@const enabled = Array.isArray(r.permissions) && r.permissions.includes(p.id)}
                    <label class="mgr-toggle-row" title={p.help}>
                      <span class="mgr-toggle-label">{p.label}</span>
                      <input type="checkbox" checked={enabled}
                        on:change={() => manageTogglePermission(selectedRole, p.id)} />
                      <span class="mgr-slider" style="--active-color: {r.color}"></span>
                    </label>
                  {/each}
                </div>
              </div>
            </div>
          {:else}
            <div style="text-align:center;padding:40px;color:var(--text3)">
              Sélectionne un rôle ci-dessus pour éditer ses accès.
            </div>
          {/if}
        {/if}

        <!-- Sticky footer : vert si dernière modif a été générée+copiée, sinon orange si dirty -->
        {#if !managerLoading && managerSaved}
          <div class="mgr-footer mgr-footer-saved">
            <div>
              <span style="font-size:16px">✅</span>
              <span style="color:#7ef0c0;font-weight:600">Modifications sauvegardées</span>
              <span style="color:var(--text3);font-size:12px;margin-left:8px">— JSON copié dans le presse-papier, colle-le sur GitHub pour activer</span>
            </div>
            <div style="display:flex;gap:8px">
              <button class="btn-test" on:click={manageGenerateAndCopy}>📋 Re-copier</button>
            </div>
          </div>
        {:else if !managerLoading && managerDirty}
          <div class="mgr-footer">
            <div>
              <span style="font-size:16px">⚠</span>
              <span style="color:var(--text2);font-weight:600">Modifications non sauvegardées</span>
              <span style="color:var(--text3);font-size:12px;margin-left:8px">— le JSON doit être collé sur GitHub pour prendre effet</span>
            </div>
            <div style="display:flex;gap:8px">
              <button class="btn-test" on:click={manageCancel}>🔄 Annuler</button>
              <button class="btn-save" on:click={manageGenerateAndCopy}>📋 Générer team.json</button>
            </div>
          </div>
        {/if}

        <!-- Modal : ajouter user -->
        {#if addUserOpen}
          <div class="mgr-modal-backdrop" on:click|self={() => addUserOpen = false}>
            <div class="mgr-modal">
              <div class="mgr-modal-header">
                <h3>➕ Ajouter un utilisateur</h3>
                <button class="mgr-icon-btn" on:click={() => addUserOpen = false}>✕</button>
              </div>
              <div class="mgr-modal-body">
                <div class="field">
                  <label for="newu-pseudo">Pseudo</label>
                  <input id="newu-pseudo" type="text" bind:value={newUserForm.pseudo}
                    autocapitalize="off" spellcheck="false" placeholder="Ex: Karo" autofocus />
                </div>
                <div class="field">
                  <label for="newu-password">Mot de passe</label>
                  <input id="newu-password" type="text" bind:value={newUserForm.password}
                    placeholder="En clair — sera hashé automatiquement" />
                </div>
                <div class="field">
                  <label for="newu-role">Rôle</label>
                  <select id="newu-role" bind:value={newUserForm.role}>
                    {#each Object.entries(teamRoles) as [k, r]}
                      <option value={k}>{r.badge} {k} — {r.title || k}</option>
                    {/each}
                  </select>
                </div>
                <div class="field">
                  <label for="newu-title">Titre (optionnel)</label>
                  <input id="newu-title" type="text" bind:value={newUserForm.title}
                    placeholder="Laisser vide = titre par défaut du rôle" />
                </div>
              </div>
              <div class="mgr-modal-footer">
                <button class="btn-test" on:click={() => addUserOpen = false}>Annuler</button>
                <button class="btn-save" on:click={manageAddUser} disabled={!newUserForm.pseudo || !newUserForm.password}>Ajouter</button>
              </div>
            </div>
          </div>
        {/if}

        <!-- Modal : éditer user -->
        {#if editUserOpen && editUserTarget}
          <div class="mgr-modal-backdrop" on:click|self={() => { editUserOpen = false; editUserTarget = null }}>
            <div class="mgr-modal">
              <div class="mgr-modal-header">
                <h3>✏️ Modifier {editUserTarget.pseudo}</h3>
                <button class="mgr-icon-btn" on:click={() => { editUserOpen = false; editUserTarget = null }}>✕</button>
              </div>
              <div class="mgr-modal-body">
                <div class="field">
                  <label for="edu-role">Rôle</label>
                  <select id="edu-role" bind:value={editUserTarget.role}
                    disabled={editUserTarget.pseudo === myUsername && editUserTarget.role === 'admin'}>
                    {#each Object.entries(teamRoles) as [k, r]}
                      <option value={k}>{r.badge} {k} — {r.title || k}</option>
                    {/each}
                  </select>
                  {#if editUserTarget.pseudo === myUsername && editUserTarget.role === 'admin'}
                    <div class="field-hint">Tu ne peux pas te retirer le rôle admin à toi-même.</div>
                  {/if}
                </div>
                <div class="field">
                  <label for="edu-title">Titre</label>
                  <input id="edu-title" type="text" bind:value={editUserTarget.title} />
                </div>
                <div class="field">
                  <label for="edu-newpass">Nouveau mot de passe (optionnel)</label>
                  <input id="edu-newpass" type="text" bind:value={editUserTarget.newPassword}
                    placeholder="Laisser vide = ne pas changer" />
                </div>
              </div>
              <div class="mgr-modal-footer">
                <button class="btn-test" on:click={() => { editUserOpen = false; editUserTarget = null }}>Annuler</button>
                <button class="btn-save" on:click={manageEditUser}>Enregistrer</button>
              </div>
            </div>
          </div>
        {/if}

        <!-- Modal : ajouter rôle -->
        {#if addRoleOpen}
          <div class="mgr-modal-backdrop" on:click|self={() => addRoleOpen = false}>
            <div class="mgr-modal">
              <div class="mgr-modal-header">
                <h3>➕ Créer un rôle</h3>
                <button class="mgr-icon-btn" on:click={() => addRoleOpen = false}>✕</button>
              </div>
              <div class="mgr-modal-body">
                <div class="field">
                  <label for="newr-slug">Slug (identifiant interne)</label>
                  <input id="newr-slug" type="text" bind:value={newRoleForm.slug}
                    autocapitalize="off" spellcheck="false" placeholder="Ex: uploader, vip, friend" autofocus />
                  <div class="field-hint">Lettres/chiffres/tirets uniquement — sert de clé dans team.json.</div>
                </div>
                <div class="field">
                  <label for="newr-title">Titre affiché</label>
                  <input id="newr-title" type="text" bind:value={newRoleForm.title}
                    placeholder="Ex: Uploader, Membre VIP" />
                </div>
                <div class="field">
                  <label>Emoji / badge</label>
                  <div class="mgr-emoji-grid">
                    {#each ROLE_EMOJIS as e}
                      <button class="mgr-emoji-btn" class:active={newRoleForm.badge === e} on:click={() => newRoleForm.badge = e}>{e}</button>
                    {/each}
                  </div>
                </div>
                <div class="field">
                  <label>Couleur</label>
                  <div class="mgr-color-grid">
                    {#each ROLE_COLORS as c}
                      <button class="mgr-color-btn" class:active={newRoleForm.color === c.hex}
                        style="background:{c.hex}" title={c.name}
                        on:click={() => newRoleForm.color = c.hex}></button>
                    {/each}
                  </div>
                </div>
                <div style="background:rgba(255,255,255,0.03);padding:12px;border-radius:10px;margin-top:6px">
                  <div style="font-size:11px;color:var(--text3);margin-bottom:6px;text-transform:uppercase;letter-spacing:0.5px">Aperçu</div>
                  <div style="display:flex;gap:10px;align-items:center">
                    <span style="font-size:22px">{newRoleForm.badge}</span>
                    <span style="font-weight:700;color:{newRoleForm.color}">{newRoleForm.slug || 'slug'}</span>
                    <span style="color:var(--text3)">— {newRoleForm.title || 'titre'}</span>
                  </div>
                </div>
              </div>
              <div class="mgr-modal-footer">
                <button class="btn-test" on:click={() => addRoleOpen = false}>Annuler</button>
                <button class="btn-save" on:click={manageAddRole} disabled={!newRoleForm.slug}>Créer</button>
              </div>
            </div>
          </div>
        {/if}

        <!-- Modal : output JSON -->
        {#if outputOpen}
          <div class="mgr-modal-backdrop" on:click|self={() => outputOpen = false}>
            <div class="mgr-modal" style="max-width:700px">
              <div class="mgr-modal-header">
                <h3>📋 team.json généré</h3>
                <button class="mgr-icon-btn" on:click={() => outputOpen = false}>✕</button>
              </div>
              <div class="mgr-modal-body">
                <div style="color:var(--text3);font-size:12px;margin-bottom:8px">
                  ✅ Le JSON complet a été copié dans ton presse-papier. Ouvre team.json sur GitHub, remplace <b>tout le contenu</b>, et commit.
                </div>
                <textarea readonly rows="18" class="mgr-output">{outputJSON}</textarea>
              </div>
              <div class="mgr-modal-footer">
                <button class="btn-test" on:click={() => { navigator.clipboard.writeText(outputJSON); }}>📋 Recopier</button>
                <button class="btn-save" on:click={() => { OpenBrowser('https://github.com/Gandalfleblanc/Go-Post-Tools-Hydracker/edit/main/team.json'); outputOpen = false }}>
                  Ouvrir sur GitHub →
                </button>
              </div>
            </div>
          </div>
        {/if}
      </div>

    <!-- ===== RÉGLAGES (fusion API + config) ===== -->
    {:else if activeTab === 'settings'}
      <div class="tab-content">
        <h2>Réglages</h2>
        <div class="sections">

          {#if myRole === 'admin'}
            <div style="background:rgba(126,240,192,0.04);border:1px solid rgba(126,240,192,0.2);border-radius:10px;padding:12px 14px;margin-bottom:16px;display:flex;align-items:center;justify-content:space-between;gap:10px">
              <div style="color:var(--text2);font-size:13px">
                👥 Gestion des utilisateurs et permissions → onglet
                <button class="mgr-inline-link" on:click={() => activeTab = 'manager'}>👥 Manager</button>
              </div>
            </div>
          {/if}

          <!-- ===== Clés API ===== -->
          <div style="font-size:11px;color:var(--text3);margin:0 0 8px;text-transform:uppercase;letter-spacing:0.5px">🔑 Clés API</div>

          <div class="section section-locked">
            <div class="section-header">
              <span>🔒 Hydracker (verrouillé team)</span>
            </div>
            <div class="field">
              <label>URL de base</label>
              <input type="password" value={cfg.hydracker_base_url} disabled readonly />
              <div class="field-hint">URL définie au build — non modifiable. Token perso → carte "Hydracker" plus bas.</div>
            </div>
          </div>

          <div class="section section-locked">
            <div class="section-header">
              <span>🔒 TMDB (verrouillé team)</span>
              <button class="btn-test" on:click={() => runTest('tmdb', () => TestTMDB(''))}>
                {#if testLoading.tmdb}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.tmdb}
              <div class="test-result" class:ok={testResults.tmdb.ok}>{testResults.tmdb.message}</div>
            {/if}
            <div style="color:var(--text3);font-size:12px;line-height:1.5;margin-bottom:8px">
              ✅ Recherche TMDB via le proxy configuré — pas de clé requise.<br>
              Bonus : notes IMDb fusionnées + lookup par IMDb ID + watch providers (Netflix/Disney+…).
            </div>
            <div class="field">
              <label>Index de recherche TMDB (proxy URL)</label>
              <input type="password" value={cfg.tmdb_proxy_url} disabled readonly />
              <div class="field-hint">URL imposée par la team — non modifiable.</div>
            </div>
            <div class="field">
              <label>Index de recherche LiHDL (par nom de fichier)</label>
              <input type="password" value={cfg.media_search_url} disabled readonly />
              <div class="field-hint">Endpoint custom imposé par la team — non modifiable.</div>
            </div>
            <div class="field">
              <label>Index de recherche TEAM</label>
              <input type="password" value={cfg.lihdl_base_url} disabled readonly />
              <div class="field-hint">Dossier LiHDL team-shared — verrouillé par la team. Clé perso TMDB → carte "TMDB" plus bas.</div>
            </div>
          </div>

          <!-- Cartes éditables : clés perso utilisateur (TMDB + Hydracker) -->
          <div class="section">
            <div class="section-header">
              <span>TMDB</span>
              <button class="btn-test" on:click={() => runTest('tmdb', () => TestTMDB(cfg.tmdb_api_key))}>
                {#if testLoading.tmdb}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.tmdb}
              <div class="test-result" class:ok={testResults.tmdb.ok}>{testResults.tmdb.message}</div>
            {/if}
            <div class="field">
              <label>Clé API TMDB</label>
              <input type="password" bind:value={cfg.tmdb_api_key} placeholder="API key" />
              <div class="field-hint">Fallback si le proxy team est down. Récupère ta clé sur themoviedb.org → Settings → API.</div>
            </div>
          </div>

          <div class="section">
            <div class="section-header">
              <span>Hydracker</span>
              <button class="btn-test" on:click={() => runTest('hydracker', () => TestHydracker(cfg.hydracker_base_url, cfg.hydracker_token))}>
                {#if testLoading.hydracker}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.hydracker}
              <div class="test-result" class:ok={testResults.hydracker.ok}>{testResults.hydracker.message}</div>
            {/if}
            <div class="field">
              <label>Token d'accès</label>
              <input type="password" bind:value={cfg.hydracker_token} placeholder="Bearer token" />
              <div class="field-hint">Chaque user met son propre token Hydracker (récupérable depuis ton profil Hydracker).</div>
            </div>
          </div>

          <div class="section">
            <div class="section-header">
              <span>1Fichier</span>
              <button class="btn-test" on:click={() => runTest('onefichier', () => TestOneFichier(cfg.one_fichier_api_key))}>
                {#if testLoading.onefichier}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.onefichier}
              <div class="test-result" class:ok={testResults.onefichier.ok}>{testResults.onefichier.message}</div>
            {/if}
            <div class="field">
              <label>Clé API 1Fichier</label>
              <input type="password" bind:value={cfg.one_fichier_api_key} placeholder="API key" />
            </div>
          </div>

          <div class="section">
            <div class="section-header">
              <span>Send.now</span>
              <button class="btn-test" on:click={() => runTest('sendcm', () => TestSendCm(cfg.sendcm_api_key))}>
                {#if testLoading.sendcm}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.sendcm}
              <div class="test-result" class:ok={testResults.sendcm.ok}>{testResults.sendcm.message}</div>
            {/if}
            <div class="field">
              <label>Clé API Send.now</label>
              <input type="password" bind:value={cfg.sendcm_api_key} placeholder="API key" />
            </div>
          </div>

          <div class="section">
            <div class="section-header">
              <span>IGDB (Jeux vidéo)</span>
              <button class="btn-test" on:click={() => runTest('igdb', async () => ({ ok: true, message: await TestIgdb(cfg.igdb_client_id, cfg.igdb_client_secret) }))}>
                {#if testLoading.igdb}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.igdb}
              <div class="test-result" class:ok={testResults.igdb.ok}>{testResults.igdb.message}</div>
            {/if}
            <div class="field">
              <label>Twitch Client ID</label>
              <input type="password" bind:value={cfg.igdb_client_id} placeholder="client id" />
            </div>
            <div class="field">
              <label>Twitch Client Secret</label>
              <input type="password" bind:value={cfg.igdb_client_secret} placeholder="client secret" />
              <div class="field-hint">Active la recherche de jeux vidéo (toggle 🎮 Jeu dans l'onglet Hydracker). Crée une app sur console.twitch.tv → Applications → Enregistrer (gratuit), puis colle le Client ID + Secret.</div>
            </div>
          </div>

          <!-- ===== Configuration ===== -->
          <div style="font-size:11px;color:var(--text3);margin:18px 0 8px;text-transform:uppercase;letter-spacing:0.5px">⚙️ Configuration</div>

          <!-- Usenet -->
          <div class="section">
            <div class="section-header">
              <span>Usenet</span>
              <button class="btn-test" on:click={() => runTest('usenet', () => TestUsenet(cfg.usenet_host, cfg.usenet_port))}>
                {#if testLoading.usenet}…{:else}Tester{/if}
              </button>
            </div>
            {#if testResults.usenet}
              <div class="test-result" class:ok={testResults.usenet.ok}>{testResults.usenet.message}</div>
            {/if}
            <div class="fields-grid">
              <div class="field">
                <label>Serveur</label>
                <input type="text" bind:value={cfg.usenet_host} placeholder="news.example.com" />
              </div>
              <div class="field">
                <label>Port</label>
                <input type="number" bind:value={cfg.usenet_port} />
              </div>
              <div class="field">
                <label>Utilisateur</label>
                <input type="text" bind:value={cfg.usenet_user} />
              </div>
              <div class="field">
                <label>Mot de passe</label>
                <input type="password" bind:value={cfg.usenet_password} />
              </div>
              <div class="field">
                <label>Connexions</label>
                <input type="number" bind:value={cfg.usenet_connections} />
              </div>
              <div class="field field-checkbox">
                <label>
                  <input type="checkbox" bind:checked={cfg.usenet_ssl} />
                  SSL/TLS
                </label>
              </div>
              <div class="field">
                <label for="usenet-group">Newsgroup</label>
                <input id="usenet-group" type="text" bind:value={cfg.usenet_group} placeholder="alt.binaries.test" />
              </div>
            </div>
          </div>

          <!-- Watch folder -->
          <div class="section">
            <div class="section-header">
              <span>Dossier surveillé (auto-post)</span>
              {#if watchRunning}
                <button class="btn-test" style="color:#ff9585" on:click={async () => { try { await StopWatchFolder() } catch(e){} }}>■ Arrêter</button>
              {:else}
                <button class="btn-test" on:click={async () => { try { await StartWatchFolder(cfg.watch_folder) } catch(e){ addLog('WATCH', '✗ ' + e) } }}>▶ Démarrer</button>
              {/if}
            </div>
            <div class="field">
              <label for="watch-folder">Chemin</label>
              <input id="watch-folder" type="text" bind:value={cfg.watch_folder} placeholder="/Users/gandalf/Desktop/LiHDL" />
              <div class="field-hint">Chaque nouveau .mkv/.mp4 dans ce dossier déclenche l'analyse (et l'auto-post si Full Auto est activé).</div>
            </div>
            <div class="field field-checkbox">
              <label>
                <input type="checkbox" bind:checked={cfg.watch_auto_start} />
                Démarrer automatiquement au lancement de l'app
              </label>
            </div>
            {#if watchRunning}
              <div class="test-result ok">● Surveillance active sur : {cfg.watch_folder}</div>
            {/if}
          </div>



          <!-- ParPar -->
          <div class="section">
            <div class="section-header"><span>ParPar (PAR2)</span></div>
            <div class="fields-grid">
              <div class="field">
                <label for="parpar-redundancy">Redondance (%)</label>
                <input id="parpar-redundancy" type="number" bind:value={cfg.parpar_redundancy} min="1" max="100" step="1" />
              </div>
              <div class="field">
                <label for="parpar-threads">Threads</label>
                <input id="parpar-threads" type="number" bind:value={cfg.parpar_threads} min="1" max="64" />
              </div>
              <div class="field">
                <label for="parpar-slice">Slice size (octets)</label>
                <input id="parpar-slice" type="number" bind:value={cfg.parpar_slice_size} step="1024" />
              </div>
            </div>
          </div>


        </div>

        <button class="btn-save" on:click={saveConfig}>
          {cfgSaved ? '✓ Sauvegardé' : 'Sauvegarder'}
        </button>
      </div>

    <!-- ===== LOGS : Journal app (sous-onglet) ===== -->
    {:else if activeTab === 'logs' && logsSubTab === 'journal'}
      <div class="tab-content">
        <h2>🔬 Logs</h2>
        <div class="sub-tabs-nav">
          <button class:active={logsSubTab === 'journal'} on:click={() => logsSubTab = 'journal'}>📋 Journal app</button>
          <button class:active={logsSubTab === 'api'} on:click={() => logsSubTab = 'api'}>🔬 Requêtes API</button>
        </div>
        {#if $logEntries.length === 0}
          <p class="coming-soon">Les logs en temps réel apparaîtront ici</p>
        {:else}
          <div class="log-toolbar">
            <button class="btn-log-clear" on:click={clearLogs}>Effacer</button>
          </div>
          <div class="log-list">
            {#each $logEntries as e}
              <div class="log-line">
                <span class="log-ts">{e.ts}</span>
                <span class="log-prefix log-prefix-{e.prefix.toLowerCase()}">{e.prefix}</span>
                <span class="log-msg">{e.msg}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}

    {/if}<!-- fin activeTab !== hydracker -->

  </main>
</div>
{/if}<!-- fin authState === 'ok' -->

<!-- ===== Modale mot de passe LiHDL ===== -->
{#if lihdlModal}
  <div class="modal-backdrop" on:click|self={() => lihdlModal = null}>
    <div class="modal-card">
      <div class="modal-header">
        <h3>
          {#if lihdlModal === 'unlock'}🔓 Déverrouiller LiHDL
          {:else if lihdlModal === 'create'}🔐 Définir un mot de passe
          {:else if lihdlModal === 'change'}🔑 Changer le mot de passe
          {:else}🗑 Retirer la protection{/if}
        </h3>
        <button class="modal-close" on:click={() => lihdlModal = null}>✕</button>
      </div>
      <div class="modal-body">
        <form on:submit|preventDefault={submitLihdlPwd}>
          {#if lihdlModal === 'unlock'}
            <div class="field"><label>Mot de passe</label>
              <input type="password" bind:value={lihdlPwdInput} autofocus autocomplete="off" /></div>
          {:else if lihdlModal === 'create'}
            <div class="field"><label>Nouveau mot de passe</label>
              <input type="password" bind:value={lihdlPwdNew} autofocus autocomplete="off" /></div>
            <div class="field"><label>Confirmer</label>
              <input type="password" bind:value={lihdlPwdConfirm} autocomplete="off" /></div>
          {:else if lihdlModal === 'change'}
            <div class="field"><label>Mot de passe actuel</label>
              <input type="password" bind:value={lihdlPwdCurrent} autofocus autocomplete="off" /></div>
            <div class="field"><label>Nouveau mot de passe</label>
              <input type="password" bind:value={lihdlPwdNew} autocomplete="off" /></div>
            <div class="field"><label>Confirmer</label>
              <input type="password" bind:value={lihdlPwdConfirm} autocomplete="off" /></div>
          {:else}
            <div class="field"><label>Mot de passe actuel</label>
              <input type="password" bind:value={lihdlPwdCurrent} autofocus autocomplete="off" /></div>
            <p class="modal-hint">La section redeviendra accessible sans mot de passe.</p>
          {/if}
          {#if lihdlPwdError}<p class="modal-err">✗ {lihdlPwdError}</p>{/if}
          <div class="modal-actions" style="margin-top:12px">
            <button type="button" class="btn-secondary" on:click={() => lihdlModal = null}>Annuler</button>
            <button type="submit" class="btn-primary">Valider</button>
          </div>
        </form>
      </div>
    </div>
  </div>
{/if}

<!-- ===== Modale mot de passe SEEDBOX (FTP+seedbox sections) ===== -->
{#if seedboxModal}
  <div class="modal-backdrop" on:click|self={() => seedboxModal = null}>
    <div class="modal-card">
      <div class="modal-header">
        <h3>
          {#if seedboxModal === 'unlock'}🔓 Déverrouiller Seedbox/FTP
          {:else if seedboxModal === 'create'}🔐 Définir un mot de passe
          {:else if seedboxModal === 'change'}🔑 Changer le mot de passe
          {:else}🗑 Retirer la protection{/if}
        </h3>
        <button class="modal-close" on:click={() => seedboxModal = null}>✕</button>
      </div>
      <div class="modal-body">
        <form on:submit|preventDefault={submitSeedboxPwd}>
          {#if seedboxModal === 'unlock'}
            <div class="field"><label>Mot de passe</label>
              <input type="password" bind:value={seedboxPwdInput} autofocus autocomplete="off" /></div>
          {:else if seedboxModal === 'create'}
            <p class="modal-hint">Mot de passe partagé entre les admins. Donne-le aux 3 autres pour qu'ils puissent déverrouiller leurs sections aussi.</p>
            <div class="field"><label>Nouveau mot de passe</label>
              <input type="password" bind:value={seedboxPwdNew} autofocus autocomplete="off" /></div>
            <div class="field"><label>Confirmer</label>
              <input type="password" bind:value={seedboxPwdConfirm} autocomplete="off" /></div>
          {:else if seedboxModal === 'change'}
            <div class="field"><label>Mot de passe actuel</label>
              <input type="password" bind:value={seedboxPwdCurrent} autofocus autocomplete="off" /></div>
            <div class="field"><label>Nouveau mot de passe</label>
              <input type="password" bind:value={seedboxPwdNew} autocomplete="off" /></div>
            <div class="field"><label>Confirmer</label>
              <input type="password" bind:value={seedboxPwdConfirm} autocomplete="off" /></div>
          {:else}
            <div class="field"><label>Mot de passe actuel</label>
              <input type="password" bind:value={seedboxPwdCurrent} autofocus autocomplete="off" /></div>
            <p class="modal-hint">Les sections redeviendront accessibles sans mot de passe.</p>
          {/if}
          {#if seedboxPwdError}<p class="modal-err">✗ {seedboxPwdError}</p>{/if}
          <div class="modal-actions" style="margin-top:12px">
            <button type="button" class="btn-secondary" on:click={() => seedboxModal = null}>Annuler</button>
            <button type="submit" class="btn-primary">Valider</button>
          </div>
        </form>
      </div>
    </div>
  </div>
{/if}

<!-- ===== Modale mise à jour ===== -->
{#if showUpdateModal && updateInfo?.available}
  <div class="modal-backdrop" on:click|self={() => { if (!updateState.downloading) showUpdateModal = false }}>
    <div class="modal-card">
      <div class="modal-header">
        <h3>🆕 Mise à jour disponible</h3>
        <button class="modal-close" on:click={() => { if (!updateState.downloading) showUpdateModal = false }}>✕</button>
      </div>
      <div class="modal-body">
        <p>Version actuelle : <b>v{updateInfo.current}</b> → nouvelle : <b>v{updateInfo.latest}</b></p>
        {#if !updateState.stage}
          <p class="modal-hint">Télécharger la nouvelle version dans ton dossier Téléchargements.</p>
        {:else if updateState.downloading}
          <div class="update-progress">
            <div class="update-bar"><div class="update-bar-fill" style="width:{updateState.percent}%"></div></div>
            <div class="update-msg">{updateState.msg} · {updateState.percent.toFixed(0)}%</div>
          </div>
        {:else if updateState.stage === 'done'}
          <p class="modal-ok">✓ Téléchargé. Le Finder s'est ouvert sur le fichier. Dézippe et remplace l'app.</p>
          <p class="modal-hint" style="margin-top:8px">Chemin : <code>{updateState.downloadedPath}</code></p>
        {:else if updateState.stage === 'error'}
          <p class="modal-err">✗ Erreur : {updateState.msg}</p>
        {/if}
      </div>
      <div class="modal-actions">
        {#if !updateState.stage || updateState.stage === 'error'}
          <button class="btn-primary" on:click={startDownloadUpdate}>⬇ Télécharger</button>
          <button class="btn-secondary" on:click={() => OpenBrowser(updateInfo.url)}>Voir sur GitHub</button>
        {:else if updateState.stage === 'done'}
          <button class="btn-primary" on:click={() => showUpdateModal = false}>Fermer</button>
        {/if}
      </div>
    </div>
  </div>
{/if}

{#if nfoModal}
  <div class="nfo-modal-bg" on:click={closeNfo}>
    <div class="nfo-modal-card" on:click|stopPropagation>
      <div class="nfo-modal-head">
        <div>
          <div class="nfo-modal-title">📄 NFO — {nfoModal.kind === 'torrents' ? 'Torrent' : nfoModal.kind === 'nzbs' ? 'NZB' : 'Lien DDL'} #{nfoModal.id}</div>
          {#if nfoModal.title}<div class="nfo-modal-sub">{nfoModal.title}</div>{/if}
        </div>
        <button class="btn-test" on:click={closeNfo}>✕ Fermer</button>
      </div>
      <div class="nfo-modal-body">
        {#if nfoModal.loading}
          <div style="color:var(--text3);font-size:12px">Chargement…</div>
        {:else if nfoModal.error}
          <div style="color:#ff9585;font-size:12px">{nfoModal.error}</div>
        {:else}
          {@html nfoModal.html}
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  :global(:root) {
    color-scheme: dark;
    --bg:         #0d0a10;
    --bg-tint:    #14101a;
    --bg2:        #1a1420;
    --bg3:        #241c2a;
    --border:     rgba(255, 255, 255, 0.08);
    --border-strong: rgba(255, 255, 255, 0.14);
    --red:        #e63946;
    --red-hot:    #ff5a4a;
    --orange:     #f77f00;
    --blue:       #00b4d8;
    --blue-hot:   #48cae4;
    --yellow:     #ffd60a;
    --accent:     var(--red);
    --accent-glow: rgba(230, 57, 70, 0.45);
    --success:    #22c55e;
    --warning:    #ffd60a;
    --danger:     #ef4444;
    --text:       #f5efe7;
    --text2:      #b5a9a1;
    --text3:      #7a6e68;
    --grad-primary:       linear-gradient(180deg, #ff6b3d 0%, #e8431a 100%);
    --grad-primary-hover: linear-gradient(180deg, #ff7a4f 0%, #f14a24 100%);
  }
  :global(*) { box-sizing: border-box; margin: 0; padding: 0; }
  :global(body) {
    margin: 0;
    font-family: "SF Pro Display", -apple-system, BlinkMacSystemFont, "Segoe UI Variable", "Segoe UI", system-ui, sans-serif;
    font-size: 14px;
    color: var(--text);
    background:
      radial-gradient(ellipse 120% 80% at 50% -10%, rgba(230, 57, 70, 0.08) 0%, transparent 50%),
      radial-gradient(ellipse 80% 60% at 20% 110%, rgba(0, 180, 216, 0.06) 0%, transparent 55%),
      var(--bg);
    height: 100vh;
    overflow: hidden;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
  }
  :global(body::before) {
    content: "";
    position: fixed;
    inset: 0;
    pointer-events: none;
    z-index: 1;
    opacity: 0.035;
    background-image: url("data:image/svg+xml;utf8,<svg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='3' stitchTiles='stitch'/></filter><rect width='100%25' height='100%25' filter='url(%23n)'/></svg>");
    mix-blend-mode: overlay;
  }
  :global(input[type=text]),
  :global(input[type=password]),
  :global(input[type=number]),
  :global(input[type=email]),
  :global(select) {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--text);
    padding: 8px 11px;
    font: inherit;
    font-size: 13px;
    outline: none;
    width: 100%;
    transition: border-color 140ms, background 140ms;
  }
  :global(input:focus), :global(select:focus) {
    border-color: rgba(0, 180, 216, 0.45);
    background: rgba(255,255,255,0.05);
  }
  :global(button) { cursor: pointer; border: none; border-radius: 8px; font-size: 13px; font-weight: 500; transition: all 140ms ease; }
  :global(::-webkit-scrollbar) { width: 6px; height: 6px; }
  :global(::-webkit-scrollbar-track) { background: transparent; }
  :global(::-webkit-scrollbar-thumb) { background: rgba(255,255,255,0.1); border-radius: 3px; }
  :global(::-webkit-scrollbar-thumb:hover) { background: rgba(255,255,255,0.2); }

  .layout { display: flex; height: 100vh; position: relative; z-index: 2; }

  .sidebar {
    width: 200px; min-width: 200px;
    background: rgba(20, 16, 26, 0.6);
    backdrop-filter: blur(12px);
    border-right: 1px solid var(--border);
    display: flex; flex-direction: column;
    padding: 18px 12px;
    position: relative;
    transition: width 0.2s ease, min-width 0.2s ease, padding 0.2s ease;
    height: 100vh;
    overflow-y: auto;
    overflow-x: hidden;
    /* Scroll à la molette mais scrollbar invisible */
    scrollbar-width: none;
  }
  .sidebar::-webkit-scrollbar { display: none; }
  .sidebar.collapsed { width: 56px; min-width: 56px; padding: 18px 6px; }
  .sidebar.collapsed .brand { margin-bottom: 14px; }
  .sidebar.collapsed .brand-logo { width: 36px; }
  .sidebar.collapsed .nav-item {
    padding: 8px 4px; font-size: 16px; text-align: center;
    white-space: nowrap; overflow: hidden;
  }
  .sidebar.collapsed .btn-update { padding: 8px 4px; font-size: 14px; }

  .sidebar-toggle {
    position: absolute; top: 14px; right: -12px; z-index: 5;
    width: 24px; height: 24px; border-radius: 50%;
    background: var(--bg2); border: 1px solid var(--border);
    color: var(--text2); cursor: pointer;
    display: flex; align-items: center; justify-content: center;
    font-size: 16px; line-height: 1; font-weight: 700;
    box-shadow: 0 2px 8px rgba(0,0,0,0.3);
  }
  .sidebar-toggle:hover { background: var(--bg3); color: var(--text); }
  .brand {
    display: flex; flex-direction: column; align-items: center;
    gap: 10px; margin-bottom: 24px; padding: 4px 6px;
  }
  .brand-logo {
    width: 78px; height: auto;
    filter: drop-shadow(0 0 14px rgba(230, 57, 70, 0.45));
    user-select: none;
    -webkit-user-drag: none;
  }
  .logo {
    font-size: 12px; font-weight: 700;
    letter-spacing: 2.4px;
    text-transform: uppercase;
    background: linear-gradient(135deg, #ff6b3d 0%, #ffd60a 55%, #00b4d8 100%);
    -webkit-background-clip: text;
    background-clip: text;
    -webkit-text-fill-color: transparent;
    user-select: none;
    text-align: center;
  }
  nav { display: flex; flex-direction: column; gap: 3px; }
  .nav-item {
    text-align: left; padding: 10px 13px; border-radius: 9px;
    background: transparent; color: var(--text2); font-size: 13px; width: 100%;
    transition: all 160ms ease;
  }
  .nav-item:hover { background: rgba(255,255,255,0.04); color: var(--text); }
  .nav-item.active {
    background: rgba(0, 180, 216, 0.08);
    color: var(--text);
    font-weight: 600;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.05);
    border: 1px solid rgba(0, 180, 216, 0.2);
  }

  .content { flex: 1; overflow-y: auto; }

  .hist-stats { display:flex; flex-wrap:wrap; gap:8px; margin-bottom:14px; }
  .hist-stat { padding:5px 12px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.08); border-radius:14px; font-size:12px; color:var(--text2); }
  .hist-stat b { color:var(--text1); margin-left:4px; }
  .hist-stat.ok b { color:#22c55e; }
  .hist-stat.err b { color:#ef4444; }
  .hist-filters { display:flex; gap:10px; margin-bottom:14px; align-items:center; flex-wrap:wrap; }
  .hist-search { flex:1; min-width:260px; padding:8px 12px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.1); border-radius:8px; color:var(--text1); font-size:13px; }
  .hist-type-btns { display:flex; gap:4px; }
  .hist-type-btns button { padding:6px 12px; background:rgba(255,255,255,0.04); border:1px solid rgba(255,255,255,0.08); border-radius:8px; color:var(--text2); font-size:12px; cursor:pointer; }
  .hist-type-btns button.active { background:rgba(255,90,60,0.15); border-color:#ff5a3c; color:#ff5a3c; }
  .hist-list { display:flex; flex-direction:column; gap:6px; max-height:calc(100vh - 260px); overflow-y:auto; }
  .hist-row { display:grid; grid-template-columns:110px 70px 1fr 80px 24px; gap:12px; padding:10px 14px; background:rgba(255,255,255,0.03); border:1px solid rgba(255,255,255,0.06); border-radius:8px; align-items:start; }
  .hist-row.err { border-color:rgba(239,68,68,0.3); background:rgba(239,68,68,0.05); }
  .hist-col-date { font-size:11px; color:var(--text3); font-family:monospace; }
  .hist-col-type { font-size:10px; font-weight:700; letter-spacing:0.5px; padding:2px 6px; border-radius:4px; text-align:center; align-self:start; }
  .hist-type-torrent { background:rgba(34,197,94,0.15); color:#22c55e; }
  .hist-type-nzb { background:rgba(59,130,246,0.15); color:#3b82f6; }
  .hist-type-ddl { background:rgba(168,85,247,0.15); color:#a855f7; }
  .hist-col-main { min-width:0; }
  .hist-title { font-size:13px; color:var(--text1); font-weight:600; }
  .hist-sub { font-size:11px; color:var(--text3); margin-top:2px; }
  .hist-links { font-size:10px; color:#00b4d8; margin-top:4px; word-break:break-all; white-space:pre-line; }
  .hist-error { font-size:11px; color:#ef4444; margin-top:4px; }
  .hist-col-id { font-size:12px; color:var(--text2); font-family:monospace; }
  .hist-del { background:transparent; border:0; color:var(--text3); cursor:pointer; font-size:14px; padding:0; }
  .hist-del:hover { color:#ef4444; }
  .hist-empty { padding:40px; text-align:center; color:var(--text3); font-size:13px; background:rgba(255,255,255,0.02); border-radius:8px; }

  .modal-backdrop { position:fixed; inset:0; background:rgba(0,0,0,0.6); backdrop-filter:blur(6px); display:flex; align-items:center; justify-content:center; z-index:9999; }
  .modal-card { background:#151119; border:1px solid rgba(255,255,255,0.1); border-radius:12px; width:min(90vw, 520px); box-shadow:0 20px 60px rgba(0,0,0,0.5); }
  .modal-header { display:flex; align-items:center; justify-content:space-between; padding:16px 20px; border-bottom:1px solid rgba(255,255,255,0.08); }
  .modal-header h3 { margin:0; font-size:16px; color:var(--text1); }
  .modal-close { background:transparent; border:0; color:var(--text3); font-size:18px; cursor:pointer; }
  .modal-close:hover { color:var(--text1); }
  .modal-body { padding:20px; color:var(--text2); font-size:13px; line-height:1.5; }
  .modal-body p { margin:0 0 8px; }
  .modal-hint { color:var(--text3); font-size:12px; }
  .modal-ok { color:#22c55e; }
  .modal-err { color:#ef4444; }
  .modal-body code { font-family:monospace; font-size:11px; background:rgba(255,255,255,0.05); padding:2px 6px; border-radius:4px; color:var(--text1); word-break:break-all; }
  .update-progress { margin-top:12px; }
  .update-bar { height:8px; background:rgba(255,255,255,0.06); border-radius:4px; overflow:hidden; }
  .update-bar-fill { height:100%; background:linear-gradient(90deg, #ff5a3c, #ff8b6b); transition:width 0.2s; }
  .update-msg { margin-top:8px; font-size:12px; color:var(--text3); font-family:monospace; }
  .modal-actions { display:flex; gap:10px; justify-content:flex-end; padding:14px 20px; border-top:1px solid rgba(255,255,255,0.08); }
  .btn-primary { background:#ff5a3c; border:0; color:white; padding:8px 18px; border-radius:8px; font-weight:600; cursor:pointer; font-size:13px; }
  .btn-primary:hover { background:#ff6b4f; }
  .btn-secondary { background:rgba(255,255,255,0.06); border:1px solid rgba(255,255,255,0.1); color:var(--text1); padding:8px 18px; border-radius:8px; cursor:pointer; font-size:13px; }
  .btn-secondary:hover { background:rgba(255,255,255,0.1); }

  .tab-content { padding: 28px 32px; max-width: 900px; }
  .tab-content h2 {
    font-size: 18px; font-weight: 700; margin-bottom: 22px;
    color: var(--text); letter-spacing: -0.01em;
  }

  .coming-soon {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px dashed var(--border);
    border-radius: 14px; padding: 48px;
    text-align: center; color: var(--text3); font-size: 14px;
  }

  .sections { display: flex; flex-direction: column; gap: 16px; }

  .section {
    position: relative;
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 18px 20px;
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.05),
      0 1px 2px rgba(0,0,0,0.4);
    animation: card-in 420ms cubic-bezier(0.16, 1, 0.3, 1) both;
  }
  @keyframes card-in {
    from { opacity: 0; transform: translateY(10px); }
    to   { opacity: 1; transform: translateY(0); }
  }
  .section-header {
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: 14px;
  }
  .section-header span {
    font-weight: 600; color: var(--text2); font-size: 11px;
    text-transform: uppercase; letter-spacing: 1.2px;
  }
  /* Carte user dans la sidebar (pseudo + rôle) */
  .user-card {
    display: flex; align-items: center; gap: 10px;
    margin: 0 12px 14px;
    padding: 10px 12px;
    background: linear-gradient(135deg, rgba(0,180,216,0.08), rgba(0,180,216,0.02));
    border: 1px solid rgba(0,180,216,0.22);
    border-radius: 10px;
  }
  .user-card.compact {
    padding: 6px;
    justify-content: center;
    margin: 0 8px 12px;
  }
  .user-avatar {
    width: 36px; height: 36px;
    border-radius: 50%;
    object-fit: cover;
    flex-shrink: 0;
    background: linear-gradient(135deg, #7c3aed, #3b82f6);
  }
  .user-avatar-initial {
    display: flex; align-items: center; justify-content: center;
    font-weight: 700; font-size: 15px; color: white;
  }
  .user-info { flex: 1; min-width: 0; }
  .user-name {
    font-size: 13px; font-weight: 600; color: var(--text);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .user-role {
    font-size: 10px; font-weight: 600;
    margin-top: 2px; padding: 1px 7px; border-radius: 8px;
    display: inline-block;
  }
  .user-role-admin {
    color: #fbbf24; /* doré */
    background: rgba(251, 191, 36, 0.12);
    border: 1px solid rgba(251, 191, 36, 0.45);
    text-shadow: 0 0 8px rgba(251, 191, 36, 0.3);
  }
  .user-role-modo {
    color: #cbd5e1; /* argent */
    background: rgba(203, 213, 225, 0.10);
    border: 1px solid rgba(203, 213, 225, 0.35);
  }
  .user-role-team {
    color: #cd7f32; /* bronze */
    background: rgba(205, 127, 50, 0.12);
    border: 1px solid rgba(205, 127, 50, 0.35);
  }
  .user-role-user {
    color: #60a5fa; /* bleu */
    background: rgba(96, 165, 250, 0.12);
    border: 1px solid rgba(96, 165, 250, 0.35);
  }

  /* Écran d'auth (loading/forbidden/error) */
  .auth-screen {
    position: fixed; inset: 0;
    display: flex; align-items: center; justify-content: center;
    background: var(--bg, #0d0a10);
    padding: 20px;
  }
  .auth-card {
    background: linear-gradient(180deg, rgba(230,57,70,0.04), rgba(0,0,0,0)) , var(--bg2, #1a1420);
    border: 1px solid var(--border-strong, rgba(255,255,255,0.14));
    border-radius: 18px;
    padding: 42px 36px 34px;
    max-width: 440px; width: 100%;
    text-align: center;
    box-shadow: 0 40px 80px -30px rgba(0,0,0,0.6), 0 0 0 1px rgba(230,57,70,0.06);
  }
  .auth-logo {
    width: 140px; height: auto; margin: 0 auto 18px;
    display: block;
    filter: drop-shadow(0 8px 32px rgba(230,57,70,0.35));
    animation: logo-in 0.6s ease-out;
  }
  @keyframes logo-in {
    from { opacity: 0; transform: scale(0.8) rotate(-6deg); }
    to   { opacity: 1; transform: scale(1) rotate(0deg); }
  }

  /* SFTP toggle custom (input caché + pill custom) */
  .sftp-toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    user-select: none;
    font-weight: normal;
    font-size: 12px;
    color: var(--text2);
  }
  .sftp-toggle input { display: none; }
  .sftp-track {
    position: relative;
    width: 28px;
    height: 16px;
    background: rgba(255, 255, 255, 0.15);
    border-radius: 9999px;
    transition: background 0.2s;
    flex: none;
  }
  .sftp-thumb {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 12px;
    height: 12px;
    background: #fff;
    border-radius: 50%;
    transition: transform 0.2s;
  }
  .sftp-toggle input:checked ~ .sftp-track {
    background: rgba(96, 165, 250, 0.7);
  }
  .sftp-toggle input:checked ~ .sftp-track .sftp-thumb {
    transform: translateX(12px);
  }
  .sftp-label { letter-spacing: 0.3px; }

  /* Section verrouillée team-shared (creds bakés au build) */
  .section-locked > .section-header {
    border-bottom: 2px solid #ff4444;
    padding-bottom: 8px;
  }
  .section-locked > .section-header span {
    color: #ff4444;
  }
  .section-locked input,
  .section-locked select,
  .section-locked textarea {
    opacity: 0.5;
    cursor: not-allowed;
    background: rgba(255, 68, 68, 0.04) !important;
    border-color: rgba(255, 68, 68, 0.2) !important;
  }
  /* Exception : les champs dans .token-unlocked restent éditables malgré
     la section-locked. Utilisé pour le Token Hydracker (perso par user). */
  .section-locked .token-unlocked input,
  .section-locked .token-unlocked select,
  .section-locked .token-unlocked textarea {
    opacity: 1;
    cursor: text;
    background: rgba(126, 240, 192, 0.04) !important;
    border-color: rgba(126, 240, 192, 0.3) !important;
  }

  .btn-test {
    background: rgba(255,255,255,0.04);
    color: var(--text2);
    border: 1px solid var(--border);
    padding: 6px 14px; font-size: 11px; font-weight: 500;
    text-transform: uppercase; letter-spacing: 0.4px;
  }
  .btn-test:hover {
    background: rgba(0, 180, 216, 0.08);
    border-color: rgba(0, 180, 216, 0.35);
    color: var(--text);
  }

  .locked-box {
    padding: 16px; text-align: center;
    background: rgba(255,255,255,0.02); border: 1px dashed rgba(255,255,255,0.1);
    border-radius: 10px; color: var(--text3); font-size: 12px;
  }

  .active-chip {
    background: rgba(0, 180, 216, 0.18) !important;
    border-color: rgba(0, 180, 216, 0.55) !important;
    color: var(--text) !important;
  }
  .fiches-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: 14px;
    margin-top: 8px;
  }
  .fiche-card {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 0;
    overflow: hidden;
    cursor: pointer;
    text-align: left;
    display: flex;
    flex-direction: column;
    transition: all 0.15s;
  }
  .fiche-card:hover {
    border-color: rgba(0, 180, 216, 0.55);
    background: rgba(0, 180, 216, 0.04);
    transform: translateY(-2px);
  }
  .fiche-card img {
    width: 100%;
    aspect-ratio: 2/3;
    object-fit: cover;
    display: block;
  }
  .fiche-no-poster {
    width: 100%; aspect-ratio: 2/3;
    display: flex; align-items: center; justify-content: center;
    font-size: 48px; color: var(--text3);
    background: rgba(255,255,255,0.02);
  }
  .fiche-info { padding: 8px 10px; }
  .fiche-name { font-size: 13px; font-weight: 600; color: var(--text); margin-bottom: 3px; line-height: 1.2; }
  .fiche-meta { font-size: 11px; color: var(--text3); }
  .fiche-id { font-size: 10px; color: var(--text3); margin-top: 2px; opacity: 0.7; }

  /* === Cartes contenu (torrents/nzbs/liens) dans la fiche === */
  .content-grid { display: flex; flex-direction: column; gap: 8px; }
  .content-card {
    background: rgba(255,255,255,0.025);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 12px;
    transition: border-color 0.15s, background 0.15s;
  }
  .content-card:hover { border-color: rgba(0, 180, 216, 0.35); background: rgba(0, 180, 216, 0.025); }
  .cc-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 6px; }
  .cc-head { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
  .cc-name { font-size: 12px; color: var(--text); font-weight: 500; word-break: break-all; line-height: 1.35; }
  .cc-name-mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; color: var(--text2, #b8b3c0); }
  .cc-sub-url { opacity: 0.7; }
  .cc-name-loading { font-size: 10.5px; color: var(--text3); font-style: italic; }
  .cc-tags { display: flex; flex-wrap: wrap; gap: 5px; }
  .cc-actions { display: flex; gap: 6px; align-items: center; flex-shrink: 0; }

  .cc-id { font-size: 10px; color: var(--text3); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  .cc-chip {
    display: inline-flex; align-items: center;
    padding: 2px 8px; border-radius: 10px;
    font-size: 10px; font-weight: 600; line-height: 1.4;
    border: 1px solid transparent;
  }
  .cc-chip-qual { background: rgba(249, 115, 22, 0.12); color: #fb923c; border-color: rgba(249, 115, 22, 0.35); }
  .cc-chip-se   { background: rgba(126, 240, 192, 0.10); color: #7ef0c0; border-color: rgba(126, 240, 192, 0.30); }
  .cc-chip-size { background: rgba(255,255,255,0.05); color: var(--text3); border-color: rgba(255,255,255,0.10); }
  .cc-chip-host { background: rgba(59, 130, 246, 0.12); color: #60a5fa; border-color: rgba(59, 130, 246, 0.30); }
  .cc-chip-seed { background: rgba(255,255,255,0.04); border-color: rgba(255,255,255,0.10); }

  .cc-tag {
    display: inline-flex; align-items: center; gap: 3px;
    padding: 2px 7px; border-radius: 4px;
    font-size: 10.5px; font-weight: 500;
    background: rgba(255,255,255,0.04);
    color: var(--text3);
    border: 1px solid rgba(255,255,255,0.06);
  }
  .cc-tag-lang   { color: #d4d0db; background: rgba(126, 240, 192, 0.06); border-color: rgba(126, 240, 192, 0.18); }
  .cc-tag-sub    { color: var(--text3); background: rgba(255,255,255,0.035); }
  .cc-tag-author { color: var(--text3); background: rgba(255,255,255,0.025); font-style: italic; }

  .btn-icon {
    min-width: 28px; padding: 4px 8px !important;
    font-size: 13px; font-weight: bold;
  }

  /* === Stats uploaders (table) === */
  .stats-header {
    display: flex; justify-content: space-between; align-items: flex-start;
    gap: 16px; margin: 8px 0 18px; flex-wrap: wrap;
  }
  .stats-title { margin: 0 0 4px; font-size: 16px; font-weight: 600; color: var(--text); }
  .stats-sub { color: var(--text3); font-size: 11.5px; line-height: 1.5; max-width: 720px; }

  .upl-table-wrap {
    background: rgba(255,255,255,0.02);
    border: 1px solid var(--border);
    border-radius: 10px;
    overflow: hidden;
  }
  .upl-table {
    width: 100%; border-collapse: collapse;
  }
  .upl-table thead tr {
    background: linear-gradient(180deg, rgba(255,255,255,0.06), rgba(255,255,255,0.02));
    border-bottom: 2px solid var(--border-strong);
  }
  .upl-table th {
    padding: 11px 12px;
    text-align: left;
    font-size: 11px; font-weight: 600; color: var(--text);
    letter-spacing: 0.3px;
    text-transform: uppercase;
    white-space: nowrap;
  }
  .upl-th-rank { width: 50px; text-align: center !important; }
  .upl-th-num { text-align: right !important; }
  .upl-th-sort { cursor: pointer; user-select: none; transition: background 0.12s; }
  .upl-th-sort:hover { background: rgba(255,255,255,0.04); }

  .upl-row {
    cursor: pointer;
    transition: background 0.12s;
    border-bottom: 1px solid rgba(255,255,255,0.04);
  }
  .upl-row:hover { background: rgba(0, 180, 216, 0.06); }
  .upl-row:last-child { border-bottom: none; }
  .upl-podium { background: rgba(255, 214, 10, 0.03); }
  .upl-podium:hover { background: rgba(255, 214, 10, 0.08); }

  .upl-table td {
    padding: 9px 12px;
    font-size: 12.5px; color: var(--text);
  }
  .upl-rank {
    text-align: center;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    color: var(--text3); font-weight: 700; font-size: 13px;
    width: 50px;
  }
  .upl-author {
    font-weight: 600;
  }
  .upl-num { text-align: right; font-variant-numeric: tabular-nums; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }
  .upl-c-torrent { color: #fb923c; }
  .upl-c-nzb     { color: #60a5fa; }
  .upl-c-ddl     { color: #7ef0c0; }
  .upl-c-total   { font-weight: 700; color: var(--text); font-size: 13px; }
  .upl-c-size    { color: var(--text3); }
  .upl-date      { color: var(--text3); font-size: 11px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; white-space: nowrap; }

  /* === Modale NFO === */
  .nfo-modal-bg {
    position: fixed; inset: 0; z-index: 1000;
    background: rgba(0,0,0,0.75);
    display: flex; align-items: center; justify-content: center;
    padding: 24px;
  }
  .nfo-modal-card {
    background: var(--bg2); border: 1px solid var(--border-strong);
    border-radius: 12px; max-width: 880px; width: 100%; max-height: 85vh;
    display: flex; flex-direction: column; overflow: hidden;
    box-shadow: 0 20px 60px rgba(0,0,0,0.5);
  }
  .nfo-modal-head {
    padding: 14px 18px; border-bottom: 1px solid var(--border);
    display: flex; align-items: center; justify-content: space-between; gap: 12px;
  }
  .nfo-modal-title { font-size: 14px; font-weight: 600; color: var(--text); }
  .nfo-modal-sub { font-size: 11px; color: var(--text3); margin-top: 3px; word-break: break-all; }
  .nfo-modal-body {
    padding: 16px 18px; overflow-y: auto; flex: 1;
    font-family: ui-monospace, SFMono-Regular, Menlo, "Courier New", monospace;
    font-size: 11.5px; color: var(--text); line-height: 1.5;
    white-space: pre-wrap;
  }
  .nfo-modal-body :global(p) { margin: 0 0 2px; }
  .nfo-modal-body :global(a) { color: #60a5fa; text-decoration: none; word-break: break-all; }
  .nfo-modal-body :global(a:hover) { text-decoration: underline; }
  .nfo-modal-body :global(img) { max-width: 100%; height: auto; }

  .req-card {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 12px;
    margin-bottom: 10px;
  }
  .req-status {
    padding: 2px 8px; border-radius: 10px; font-size: 10px;
    text-transform: uppercase; letter-spacing: 0.5px; font-weight: 600;
  }
  .req-status-pending { background: rgba(255, 214, 10, 0.15); color: var(--yellow); border: 1px solid rgba(255, 214, 10, 0.35); }
  .req-status-done    { background: rgba(126, 240, 192, 0.12); color: #7ef0c0; border: 1px solid rgba(126, 240, 192, 0.35); }
  .req-status-rejected{ background: rgba(255, 107, 107, 0.12); color: #ff6b6b; border: 1px solid rgba(255, 107, 107, 0.35); }

  .req-progress {
    margin-top: 10px;
    padding-top: 10px;
    border-top: 1px solid rgba(255,255,255,0.05);
  }
  .req-progress .progress-bar {
    height: 8px;
    background: rgba(255,255,255,0.06);
    border-radius: 4px;
    overflow: hidden;
  }
  .req-progress .progress-fill {
    height: 100%;
    background: linear-gradient(90deg, #00b4d8, #7ef0c0);
    transition: width 0.2s;
    box-shadow: 0 0 10px rgba(0,180,216,0.4);
  }
  .req-progress .progress-fill.done {
    background: #7ef0c0;
    box-shadow: 0 0 10px rgba(126,240,192,0.5);
  }

  .my-item {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    margin-bottom: 6px;
    display: flex;
    gap: 12px;
    align-items: center;
  }

  .modal-backdrop {
    position: fixed; inset: 0;
    background: rgba(0,0,0,0.6);
    backdrop-filter: blur(4px);
    display: flex; align-items: center; justify-content: center;
    z-index: 1000;
  }
  .modal-card {
    background: var(--paper, #1e1e24);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px 24px;
    max-width: 480px;
    width: 90%;
    max-height: 85vh;
    overflow-y: auto;
  }
  .modal-title { font-size: 15px; font-weight: 600; color: var(--text); margin-bottom: 4px; }
  .modal-hint { color: var(--text3); font-size: 12px; margin-bottom: 12px; }

  .stat-cell {
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 8px 10px;
  }
  .stat-k { font-size: 10px; color: var(--text3); text-transform: uppercase; letter-spacing: 0.5px; margin-bottom: 4px; }
  .stat-v { font-size: 14px; font-weight: 600; color: var(--text); }

  .stat-bar {
    height: 8px;
    background: rgba(255,255,255,0.05);
    border-radius: 4px;
    overflow: hidden;
  }
  .stat-bar-fill {
    height: 100%;
    background: #00b4d8;
    transition: width 0.3s;
  }

  .api-log-row {
    padding: 8px 10px;
    border-bottom: 1px solid rgba(255,255,255,0.04);
    cursor: pointer;
    transition: background 0.1s;
  }
  .api-log-row:hover { background: rgba(255,255,255,0.02); }
  .api-log-err { background: rgba(255,107,107,0.04); }
  .api-log-method {
    display: inline-block;
    padding: 1px 8px;
    border-radius: 4px;
    font-weight: 700;
    font-size: 10px;
    letter-spacing: 0.5px;
    min-width: 50px;
    text-align: center;
  }
  .api-log-method-GET    { background: rgba(126,240,192,0.18); color: #7ef0c0; }
  .api-log-method-POST   { background: rgba(0,180,216,0.18);   color: #00b4d8; }
  .api-log-method-PUT    { background: rgba(255,214,10,0.18);  color: #ffd60a; }
  .api-log-method-PATCH  { background: rgba(255,214,10,0.18);  color: #ffd60a; }
  .api-log-method-DELETE { background: rgba(255,107,107,0.18); color: #ff6b6b; }

  .test-result {
    font-size: 12px; padding: 7px 11px; border-radius: 8px; margin-bottom: 12px;
    background: rgba(239, 68, 68, 0.08); color: #ff9585;
    border: 1px solid rgba(239, 68, 68, 0.25);
  }
  .test-result.ok {
    background: rgba(34, 197, 94, 0.08); color: #7ef0c0;
    border-color: rgba(34, 197, 94, 0.25);
  }

  .field { margin-bottom: 12px; }
  .field:last-child { margin-bottom: 0; }
  .field label { display: block; font-size: 11px; color: var(--text3); margin-bottom: 6px; text-transform: uppercase; letter-spacing: 0.6px; }
  .field-checkbox label { display: flex; align-items: center; gap: 8px; cursor: pointer; font-size: 13px; color: var(--text); margin-top: 8px; text-transform: none; letter-spacing: 0; }
  .field-hint { font-size: 11px; color: var(--text3); margin-top: 4px; }
  .pwd-row { display: flex; gap: 6px; align-items: stretch; }
  .pwd-row input { flex: 1; }
  .pwd-toggle {
    background: rgba(255,255,255,0.04); border: 1px solid var(--border);
    padding: 0 12px; font-size: 14px; border-radius: 8px;
    color: var(--text2); flex: none;
  }
  .pwd-toggle:hover { background: rgba(0, 180, 216, 0.08); border-color: rgba(0, 180, 216, 0.35); }
  .field-checkbox input[type=checkbox] { width: auto; }

  .fields-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; }

  .btn-save {
    margin-top: 24px;
    color: #fff;
    background: var(--grad-primary);
    border: 1px solid rgba(0,0,0,0.25);
    padding: 11px 28px; font-size: 14px; font-weight: 600;
    letter-spacing: 0.2px;
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.25),
      inset 0 -1px 0 rgba(0,0,0,0.2),
      0 1px 2px rgba(0,0,0,0.4),
      0 8px 24px -6px var(--accent-glow);
  }
  .btn-save:hover {
    background: var(--grad-primary-hover);
    filter: brightness(1.05);
    box-shadow:
      inset 0 1px 0 rgba(255,255,255,0.3),
      inset 0 -1px 0 rgba(0,0,0,0.2),
      0 2px 6px rgba(0,0,0,0.5),
      0 12px 32px -4px var(--accent-glow);
  }
  .btn-save:active { transform: translateY(1px); filter: brightness(0.95); }

  /* NZB live */
  .nzb-live { display: flex; flex-direction: column; gap: 14px; max-width: 600px; }
  .nzb-live-status {
    font-size: 14px; font-weight: 600; color: var(--blue-hot);
    padding: 11px 15px;
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border); border-radius: 10px;
  }
  .nzb-live-status.done { color: #7ef0c0; border-color: rgba(34, 197, 94, 0.3); background: rgba(34, 197, 94, 0.05); }
  .nzb-live-step { display: flex; align-items: center; gap: 10px; }
  .nzb-live-step > span:first-child { width: 60px; font-size: 11px; color: var(--text3); flex: none; text-transform: uppercase; letter-spacing: 0.6px; }
  .nzb-live-step .progress-bar { flex: 1; height: 8px; background: rgba(255,255,255,0.06); border-radius: 4px; overflow: hidden; }
  .nzb-live-step .progress-fill { height: 100%; background: var(--grad-primary); border-radius: 4px; transition: width 0.3s; box-shadow: 0 0 12px rgba(230, 57, 70, 0.3); }
  .nzb-live-step .pct { width: 40px; text-align: right; font-size: 12px; color: var(--text2); flex: none; font-variant-numeric: tabular-nums; }
  .nzb-live-meta { display: flex; gap: 16px; font-size: 12px; color: var(--text3); }
  .nzb-live-result { padding: 11px 15px; border-radius: 10px; font-size: 13px; font-weight: 500; }
  .nzb-live-result.ok {
    background: rgba(34, 197, 94, 0.08);
    border: 1px solid rgba(34, 197, 94, 0.25);
    color: #7ef0c0;
  }
  .nzb-live-result:not(.ok) {
    background: rgba(239, 68, 68, 0.08);
    border: 1px solid rgba(239, 68, 68, 0.25);
    color: #ff9585;
  }

  /* Journal */
  .log-toolbar { display: flex; justify-content: flex-end; margin-bottom: 10px; }
  .btn-log-clear {
    background: rgba(255,255,255,0.04);
    border: 1px solid var(--border); color: var(--text2);
    padding: 6px 14px; font-size: 11px; font-weight: 500;
    text-transform: uppercase; letter-spacing: 0.4px;
  }
  .btn-log-clear:hover { color: var(--red-hot); border-color: rgba(239, 68, 68, 0.3); }
  .log-list { display: flex; flex-direction: column; gap: 3px; font-size: 12px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  .log-line {
    display: flex; gap: 10px; align-items: baseline;
    padding: 5px 10px;
    background: rgba(255,255,255,0.02);
    border: 1px solid var(--border);
    border-radius: 6px;
  }
  .log-ts { color: var(--text3); flex: none; }
  .log-prefix { flex: none; font-weight: 700; padding: 2px 7px; border-radius: 4px; font-size: 10px; letter-spacing: 0.4px; }
  .log-prefix-ddl { background: rgba(0, 180, 216, 0.15); color: var(--blue-hot); }
  .log-prefix-nzb { background: rgba(247, 127, 0, 0.15); color: var(--orange); }
  .log-prefix-mi  { background: rgba(34, 197, 94, 0.15); color: #7ef0c0; }
  .log-prefix-meta{ background: rgba(255, 214, 10, 0.15); color: var(--yellow); }
  .log-prefix-tor { background: rgba(230, 57, 70, 0.18); color: var(--red-hot); }
  .log-msg { color: var(--text); word-break: break-all; }
  .log-prefix-chk { background: rgba(72, 202, 228, 0.18); color: var(--blue-hot); }
  .log-prefix-res { background: rgba(247, 127, 0, 0.15); color: var(--orange); }
  .log-prefix-watch { background: rgba(255, 214, 10, 0.18); color: var(--yellow); }
  .log-prefix-queue { background: rgba(255, 214, 10, 0.22); color: #ffe066; font-weight: 700; }
  .brand-version {
    font-size: 10px; color: var(--text3);
    font-family: ui-monospace, Menlo, monospace;
    letter-spacing: 0.6px; user-select: none;
  }
  .brand-author {
    font-size: 9px; color: var(--text3); opacity: 0.75;
    letter-spacing: 1.2px; text-transform: uppercase;
    margin-top: 2px; user-select: none;
  }
  .sidebar-footer { margin-top: auto; padding-top: 14px; }
  .btn-update {
    width: 100%;
    background: var(--grad-primary);
    color: #fff;
    border: 1px solid rgba(0,0,0,0.25);
    padding: 9px 12px;
    font-size: 11px; font-weight: 700;
    letter-spacing: 0.5px;
    border-radius: 8px;
    cursor: pointer;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.25), 0 6px 18px -6px var(--accent-glow);
    animation: pulse-update 2.4s ease-in-out infinite;
  }
  .btn-update:hover { filter: brightness(1.1); }

  .btn-check-update {
    width: 100%;
    background: rgba(255,255,255,0.04);
    color: var(--text3);
    border: 1px solid rgba(255,255,255,0.08);
    padding: 7px 10px;
    font-size: 11px; font-weight: 500;
    border-radius: 8px;
    cursor: pointer;
    display: flex; align-items: center; gap: 6px; justify-content: center;
  }
  .btn-check-update:hover { background: rgba(255,255,255,0.08); color: var(--text); }
  .btn-check-update:disabled { opacity: 0.5; cursor: not-allowed; }
  .sidebar.collapsed .btn-check-update { padding: 7px 4px; }

  .btn-logout {
    width: 100%;
    background: rgba(255, 107, 107, 0.06);
    color: #ff9585;
    border: 1px solid rgba(255, 107, 107, 0.18);
    padding: 7px 10px;
    font-size: 11px; font-weight: 500;
    border-radius: 8px;
    cursor: pointer;
    margin-top: 6px;
    display: flex; align-items: center; gap: 6px; justify-content: center;
  }
  .btn-logout:hover { background: rgba(255, 107, 107, 0.14); color: #ffb8b8; }
  .sidebar.collapsed .btn-logout { padding: 7px 4px; }

  .nexum-hero {
    text-align: center;
    padding: 56px 20px 40px;
    background: radial-gradient(ellipse at center, rgba(167,139,250,0.08) 0%, rgba(0,0,0,0) 60%);
    border: 1px solid var(--border);
    border-radius: 18px;
  }
  .nexum-logo-wrapper {
    position: relative;
    width: 220px; height: 220px;
    margin: 0 auto 28px;
    display: flex; align-items: center; justify-content: center;
    filter: drop-shadow(0 18px 48px rgba(167,139,250,0.25));
    animation: nexum-float 4s ease-in-out infinite;
  }
  .nexum-logo {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
    /* Masque le petit élément gris en bas à droite de l'image */
    -webkit-mask-image: linear-gradient(to bottom right, #000 70%, transparent 92%);
    mask-image: linear-gradient(to bottom right, #000 70%, transparent 92%);
  }
  @keyframes nexum-float {
    0%, 100% { transform: translateY(0); }
    50% { transform: translateY(-8px); }
  }

  .check-pagination {
    display: flex; align-items: center; justify-content: center; gap: 14px;
    margin: 14px 0 6px;
    padding: 10px;
    background: rgba(255,255,255,0.02);
    border: 1px solid var(--border);
    border-radius: 10px;
  }

  /* Mes torrents Hydracker — vue compacte */
  .myseed-header {
    display: grid;
    grid-template-columns: 44px 60px 60px 1fr 180px 100px;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    background: rgba(255,255,255,0.02);
    border-bottom: 1px solid var(--border);
    font-size: 11px;
    color: var(--text3);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    font-weight: 600;
  }
  .myseed-row-selectable {
    display: grid !important;
    grid-template-columns: 44px 60px 60px 1fr 180px 100px !important;
  }
  .myseed-row-selectable.selected { background: rgba(126,240,192,0.04); }

  .myseeds-list {
    background: var(--bg2);
    border: 1px solid var(--border);
    border-radius: 12px;
    overflow: hidden;
    margin-top: 6px;
  }
  .myseed-row {
    display: grid;
    grid-template-columns: 50px 60px 1fr 180px 180px;
    align-items: center;
    gap: 10px;
    padding: 7px 12px;
    border-bottom: 1px solid var(--border);
    transition: background 0.12s ease;
    font-size: 12px;
  }
  .myseed-row:last-child { border-bottom: none; }
  .myseed-row:hover { background: rgba(255,255,255,0.03); }
  .myseed-seeds {
    font-weight: 700; font-size: 14px;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }
  .myseed-id { color: var(--text3); font-size: 11px; font-weight: 500; }
  .myseed-name {
    color: var(--text); font-weight: 500;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .myseed-meta {
    color: var(--text3); font-size: 11px;
    text-align: right;
    white-space: nowrap;
  }
  .myseed-actions {
    display: flex; gap: 4px; justify-content: flex-end;
  }
  .myseed-btn {
    background: rgba(255,255,255,0.04);
    border: 1px solid var(--border);
    color: var(--text2);
    width: 30px; height: 30px;
    padding: 0;
    border-radius: 7px;
    cursor: pointer;
    font-size: 13px;
    display: inline-flex; align-items: center; justify-content: center;
    transition: all 0.15s ease;
  }
  .myseed-btn:hover:not(:disabled) { background: rgba(255,255,255,0.08); color: var(--text); transform: translateY(-1px); }
  .myseed-btn:disabled { opacity: 0.4; cursor: not-allowed; }
  .myseed-btn.primary {
    background: rgba(126,240,192,0.12);
    border-color: rgba(126,240,192,0.3);
    color: #7ef0c0;
  }
  .myseed-btn.primary:hover:not(:disabled) { background: rgba(126,240,192,0.2); }
  .myseed-btn.danger:hover:not(:disabled) {
    background: rgba(255,107,107,0.15);
    border-color: rgba(255,107,107,0.3);
    color: #ff6b6b;
  }

  /* Section repliable */
  .section-header-toggle {
    cursor: pointer;
    user-select: none;
    transition: background 0.15s ease;
    padding: 8px 10px; margin: -8px -10px;
    border-radius: 8px;
  }
  .section-header-toggle:hover { background: rgba(255,255,255,0.03); }
  .section-chevron {
    display: inline-block;
    color: var(--text3);
    font-size: 10px;
    transition: transform 0.2s ease;
    width: 12px; text-align: center;
  }
  .section-chevron.open { transform: rotate(90deg); color: var(--accent, #7ef0c0); }
  .section-collapsed { padding-bottom: 8px; }

  /* Toolbar Check Torrent */
  .chk-toolbar {
    display: flex; gap: 12px; align-items: center;
    margin-bottom: 14px;
    flex-wrap: wrap;
  }
  .chk-toolbar-left { display: flex; gap: 6px; flex: 1; flex-wrap: wrap; }
  .chk-toolbar-right { display: flex; gap: 10px; align-items: center; }
  .chk-sort-label { display: flex; align-items: center; gap: 8px; }
  .chk-sort-select {
    background: var(--bg2);
    border: 1px solid var(--border);
    color: var(--text);
    font-size: 12px; font-weight: 500;
    padding: 6px 12px;
    border-radius: 8px;
    cursor: pointer;
  }

  /* Bulk action bar */
  .chk-bulk-bar {
    display: flex; justify-content: space-between; align-items: center;
    flex-wrap: wrap; gap: 12px;
    background: linear-gradient(90deg, rgba(126,240,192,0.08), rgba(126,240,192,0.02));
    border: 1px solid rgba(126,240,192,0.3);
    border-radius: 12px;
    padding: 12px 16px;
    margin-bottom: 14px;
    animation: bulk-slide 0.25s ease-out;
  }
  @keyframes bulk-slide { from { opacity: 0; transform: translateY(-6px); } to { opacity: 1; transform: translateY(0); } }
  .chk-bulk-count {
    background: var(--accent, #7ef0c0);
    color: #0d0a10;
    font-weight: 700;
    padding: 2px 12px; border-radius: 12px;
    margin-right: 8px;
    font-size: 13px;
  }
  .btn-bulk-delete {
    background: linear-gradient(180deg, #ef4444, #dc2626);
    color: #fff;
    border: 1px solid rgba(0,0,0,0.25);
    padding: 8px 16px; font-size: 12px; font-weight: 700;
    border-radius: 8px;
    cursor: pointer;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.18), 0 4px 12px -4px rgba(220,38,38,0.5);
    transition: all 0.15s ease;
  }
  .btn-bulk-delete:hover:not(:disabled) { filter: brightness(1.1); transform: translateY(-1px); }
  .btn-bulk-delete:disabled { opacity: 0.5; cursor: not-allowed; }

  /* Table compacte */
  .chk-table {
    background: var(--bg2);
    border: 1px solid var(--border);
    border-radius: 12px;
    overflow: hidden;
  }
  .chk-thead {
    display: grid;
    grid-template-columns: 44px 80px 90px 1fr 60px 100px;
    align-items: center;
    padding: 10px 14px;
    background: rgba(255,255,255,0.02);
    border-bottom: 1px solid var(--border);
    font-size: 11px;
    color: var(--text3);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    font-weight: 600;
  }
  .chk-row {
    border-bottom: 1px solid var(--border);
    transition: background 0.12s ease;
  }
  .chk-row:last-child { border-bottom: none; }
  .chk-row:hover { background: rgba(255,255,255,0.02); }
  .chk-row.selected { background: rgba(126,240,192,0.04); }
  .chk-row.selected:hover { background: rgba(126,240,192,0.07); }
  .chk-row.expanded { background: rgba(255,255,255,0.03); }
  .chk-row-main {
    display: grid;
    grid-template-columns: 44px 80px 90px 1fr 60px 100px;
    align-items: center;
    padding: 10px 14px;
    cursor: pointer;
  }
  .chk-cell-check {
    display: flex; align-items: center; justify-content: center;
    cursor: pointer;
  }
  .chk-cell-check input[type="checkbox"] {
    appearance: none; -webkit-appearance: none;
    position: absolute;
    width: 0; height: 0; opacity: 0; pointer-events: none;
  }
  .chk-checkbox-custom {
    display: inline-block;
    width: 18px; height: 18px;
    background: rgba(255,255,255,0.04);
    border: 1.5px solid var(--border-strong, rgba(255,255,255,0.2));
    border-radius: 5px;
    position: relative;
    transition: all 0.15s ease;
  }
  .chk-cell-check:hover .chk-checkbox-custom { border-color: var(--accent, #7ef0c0); }
  .chk-cell-check input[type="checkbox"]:checked + .chk-checkbox-custom {
    background: var(--accent, #7ef0c0);
    border-color: var(--accent, #7ef0c0);
  }
  .chk-cell-check input[type="checkbox"]:checked + .chk-checkbox-custom::after {
    content: ''; position: absolute;
    top: 2px; left: 5px;
    width: 4px; height: 8px;
    border: solid #0d0a10;
    border-width: 0 2px 2px 0;
    transform: rotate(45deg);
  }
  .chk-cell-seeds {
    font-size: 13px; font-weight: 600;
  }
  .chk-seeds-chip {
    display: inline-flex; align-items: center; justify-content: center;
    min-width: 36px; padding: 3px 10px;
    border-radius: 12px;
    font-size: 12px; font-weight: 700;
    background: rgba(255,255,255,0.06);
    color: var(--text2);
  }
  .chk-seeds-chip.zero { background: rgba(239,68,68,0.15); color: #ff6b6b; }
  .chk-seeds-chip.low { background: rgba(251,191,36,0.15); color: #ffd60a; }
  .chk-seeds-chip.mid { background: rgba(96,165,250,0.15); color: #60a5fa; }
  .chk-seeds-chip.high { background: rgba(126,240,192,0.15); color: #7ef0c0; }
  .chk-cell-size {
    font-size: 12px; color: var(--text2);
    font-variant-numeric: tabular-nums;
    font-weight: 500;
  }
  .chk-cell-name {
    font-size: 13px; color: var(--text);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
    padding-right: 12px;
  }
  .chk-cell-status { display: flex; justify-content: center; }
  .chk-cell-status .chk-badge {
    width: 28px; height: 28px;
    border-radius: 50%;
    display: flex; align-items: center; justify-content: center;
    font-size: 12px;
    background: rgba(255,255,255,0.06);
  }
  .chk-cell-status .chk-badge.ok { background: rgba(126,240,192,0.15); color: #7ef0c0; }
  .chk-cell-status .chk-badge.err { background: rgba(255,107,107,0.15); color: #ff6b6b; }
  .chk-cell-actions {
    display: flex; gap: 4px; align-items: center; justify-content: flex-end;
  }
  .chk-chevron {
    color: var(--text3); font-size: 16px;
    transition: transform 0.2s ease;
    width: 16px; text-align: center;
  }
  .chk-chevron.open { transform: rotate(90deg); color: var(--accent, #7ef0c0); }

  .chk-row-detail {
    padding: 14px 18px 16px 58px;
    background: rgba(0,0,0,0.15);
    border-top: 1px solid var(--border);
    animation: detail-slide 0.2s ease-out;
  }
  @keyframes detail-slide { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: translateY(0); } }
  .chk-detail-row {
    display: flex; gap: 14px; padding: 4px 0;
    font-size: 12px;
  }
  .chk-detail-label {
    min-width: 130px;
    color: var(--text3);
    text-transform: uppercase;
    font-size: 10px; letter-spacing: 0.5px;
    font-weight: 600;
    padding-top: 2px;
  }
  .chk-detail-val { color: var(--text); flex: 1; }
  .chk-detail-val.mono { font-family: 'SF Mono', Monaco, monospace; font-size: 11px; color: var(--text2); word-break: break-all; }

  /* Sections dépliables par saison (Fiches séries) */
  .season-group {
    margin-bottom: 12px;
    border-radius: 10px;
    overflow: hidden;
    border: 1px solid var(--border);
  }
  .season-summary {
    display: flex; align-items: center; gap: 12px;
    padding: 10px 14px;
    background: rgba(255,255,255,0.03);
    cursor: pointer;
    list-style: none;
    user-select: none;
    transition: background 0.15s ease;
  }
  .season-summary::-webkit-details-marker { display: none; }
  .season-summary:hover { background: rgba(255,255,255,0.06); }
  .season-chevron {
    font-size: 10px; color: var(--text3);
    transition: transform 0.2s ease;
    display: inline-block;
  }
  .season-group[open] .season-chevron { transform: rotate(90deg); }
  .season-title { font-weight: 700; font-size: 14px; color: var(--text); }
  .season-count {
    font-size: 11px; color: var(--text3);
    background: rgba(255,255,255,0.06);
    padding: 2px 10px; border-radius: 10px;
    margin-left: auto;
  }
  .season-group > .content-grid { padding: 12px; }

  /* Sous-onglets (Reseed, Logs) */
  .sub-tabs-nav {
    display: flex; gap: 4px;
    margin: -6px 0 18px;
    padding: 4px;
    background: rgba(255,255,255,0.03);
    border-radius: 10px;
    width: fit-content;
  }
  .sub-tabs-nav button {
    background: transparent; border: 0;
    color: var(--text3);
    padding: 7px 14px; font-size: 12px; font-weight: 600;
    border-radius: 7px; cursor: pointer;
    transition: all 0.2s ease;
  }
  .sub-tabs-nav button:hover { color: var(--text); background: rgba(255,255,255,0.04); }
  .sub-tabs-nav button.active {
    background: var(--bg2);
    color: var(--text);
    box-shadow: 0 2px 8px -2px rgba(0,0,0,0.4), 0 0 0 1px var(--border);
  }

  /* ===== Manager tab ===== */
  .manager-tabs {
    display: flex; gap: 4px;
    margin-bottom: 18px;
    padding: 4px;
    background: rgba(255,255,255,0.03);
    border-radius: 12px;
    width: fit-content;
  }
  .manager-tabs button {
    background: transparent; border: 0;
    color: var(--text3);
    padding: 9px 18px; font-size: 13px; font-weight: 600;
    border-radius: 9px; cursor: pointer;
    display: flex; align-items: center; gap: 8px;
    transition: all 0.2s ease;
  }
  .manager-tabs button:hover { color: var(--text); background: rgba(255,255,255,0.04); }
  .manager-tabs button.active {
    background: var(--bg2);
    color: var(--text);
    box-shadow: 0 2px 10px -2px rgba(0,0,0,0.4), 0 0 0 1px var(--border);
  }
  .manager-tabs .count {
    background: rgba(255,255,255,0.08);
    color: var(--text2);
    padding: 1px 8px; border-radius: 12px;
    font-size: 11px; font-weight: 700;
    min-width: 20px; text-align: center;
  }

  .mgr-alert {
    background: rgba(255,107,107,0.08);
    border: 1px solid rgba(255,107,107,0.3);
    color: #ff9585;
    padding: 10px 14px;
    border-radius: 10px;
    font-size: 13px;
    margin-bottom: 16px;
    display: flex; justify-content: space-between; align-items: center;
  }
  .mgr-alert-close {
    background: none; border: none; color: #ff9585; cursor: pointer;
    font-size: 14px; padding: 0 4px;
  }

  /* Users grid */
  .mgr-users-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 12px;
  }
  .mgr-user-card {
    position: relative;
    background: var(--bg2);
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 16px;
    display: flex; align-items: center; gap: 14px;
    transition: all 0.2s ease;
  }
  .mgr-user-card:hover {
    border-color: var(--border-strong);
    transform: translateY(-1px);
    box-shadow: 0 8px 24px -12px rgba(0,0,0,0.5);
  }
  .mgr-user-avatar {
    width: 44px; height: 44px;
    border-radius: 50%;
    display: flex; align-items: center; justify-content: center;
    font-size: 18px; font-weight: 700;
    flex-shrink: 0;
  }
  .mgr-user-info { flex: 1; min-width: 0; }
  .mgr-user-name {
    font-size: 15px; font-weight: 700; color: var(--text);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
    display: flex; align-items: center; gap: 8px;
  }
  .mgr-user-sub { font-size: 12px; margin-top: 2px; }
  .mgr-you {
    background: rgba(126,240,192,0.15);
    color: #7ef0c0;
    font-size: 9px; font-weight: 700;
    padding: 2px 6px; border-radius: 4px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  .mgr-user-actions {
    display: flex; gap: 4px;
    opacity: 0; transition: opacity 0.2s ease;
  }
  .mgr-user-card:hover .mgr-user-actions { opacity: 1; }
  .mgr-icon-btn {
    background: rgba(255,255,255,0.04);
    border: 1px solid var(--border);
    color: var(--text2);
    width: 30px; height: 30px;
    border-radius: 8px;
    cursor: pointer;
    font-size: 14px;
    display: flex; align-items: center; justify-content: center;
    transition: all 0.2s ease;
  }
  .mgr-icon-btn:hover:not(:disabled) { background: rgba(255,255,255,0.08); color: var(--text); }
  .mgr-icon-btn:disabled { opacity: 0.3; cursor: not-allowed; }
  .mgr-icon-danger:hover:not(:disabled) {
    background: rgba(255,107,107,0.12) !important;
    color: #ff6b6b !important;
    border-color: rgba(255,107,107,0.3) !important;
  }
  .mgr-user-badge {
    position: absolute; top: -6px; right: 10px;
    background: var(--bg);
    color: #7ef0c0;
    border: 1px solid rgba(126,240,192,0.4);
    font-size: 9px; font-weight: 700;
    padding: 2px 8px; border-radius: 20px;
    text-transform: uppercase; letter-spacing: 0.5px;
  }

  .mgr-add-card {
    background: rgba(255,255,255,0.02);
    border: 2px dashed var(--border);
    color: var(--text3);
    border-radius: 14px;
    padding: 16px;
    min-height: 76px;
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    gap: 6px;
    cursor: pointer;
    font-size: 12px;
    transition: all 0.2s ease;
  }
  .mgr-add-card:hover {
    background: rgba(126,240,192,0.04);
    border-color: rgba(126,240,192,0.4);
    color: #7ef0c0;
  }

  /* Role chips */
  .mgr-role-chips {
    display: flex; flex-wrap: wrap; gap: 8px;
    margin-bottom: 18px;
  }
  .mgr-role-chip {
    position: relative;
    display: flex; align-items: center; gap: 8px;
    background: rgba(255,255,255,0.04);
    border: 1.5px solid transparent;
    color: var(--text2);
    padding: 8px 14px;
    border-radius: 20px;
    cursor: pointer;
    font-size: 13px;
    transition: all 0.15s ease;
  }
  .mgr-role-chip:hover { transform: translateY(-1px); }
  .mgr-role-chip.active { font-weight: 700; }
  .mgr-chip-del {
    margin-left: 4px;
    width: 18px; height: 18px;
    border-radius: 50%;
    background: rgba(255,107,107,0.2);
    color: #ff6b6b;
    display: none; align-items: center; justify-content: center;
    font-size: 12px; font-weight: 700;
    cursor: pointer;
  }
  .mgr-role-chip:hover .mgr-chip-del { display: flex; }
  .mgr-chip-del:hover { background: rgba(255,107,107,0.35); }
  .mgr-add-chip {
    background: rgba(126,240,192,0.06);
    color: #7ef0c0;
    border: 1.5px dashed rgba(126,240,192,0.3);
  }
  .mgr-add-chip:hover {
    background: rgba(126,240,192,0.12);
    border-color: rgba(126,240,192,0.6);
  }

  /* Role detail */
  .mgr-role-detail {
    background: var(--bg2);
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 22px;
  }
  .mgr-role-header {
    display: flex; justify-content: space-between; align-items: center;
    gap: 12px; flex-wrap: wrap;
    margin-bottom: 20px;
    padding-bottom: 16px;
    border-bottom: 1px solid var(--border);
  }
  .mgr-role-emoji {
    width: 48px; height: 48px;
    border-radius: 12px;
    display: flex; align-items: center; justify-content: center;
    font-size: 24px;
    flex-shrink: 0;
  }
  .mgr-role-tabs {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 8px;
  }

  /* Toggle switch */
  .mgr-toggle-row {
    display: flex; align-items: center; gap: 10px;
    padding: 10px 14px;
    background: rgba(255,255,255,0.02);
    border: 1px solid var(--border);
    border-radius: 10px;
    cursor: pointer;
    transition: all 0.15s ease;
    user-select: none;
  }
  .mgr-toggle-row:hover { background: rgba(255,255,255,0.04); border-color: var(--border-strong); }
  .mgr-toggle-label { flex: 1; font-size: 13px; color: var(--text2); }
  .mgr-toggle-row input[type="checkbox"] { display: none; }
  .mgr-slider {
    position: relative;
    width: 36px; height: 20px;
    background: rgba(255,255,255,0.1);
    border-radius: 999px;
    transition: all 0.2s ease;
    flex-shrink: 0;
  }
  .mgr-slider::after {
    content: '';
    position: absolute;
    top: 2px; left: 2px;
    width: 16px; height: 16px;
    background: var(--text2);
    border-radius: 50%;
    transition: all 0.2s ease;
  }
  .mgr-toggle-row input[type="checkbox"]:checked ~ .mgr-slider {
    background: var(--active-color, #7ef0c0);
  }
  .mgr-toggle-row input[type="checkbox"]:checked ~ .mgr-slider::after {
    left: 18px;
    background: #fff;
  }
  .mgr-toggle-row input[type="checkbox"]:checked + .mgr-toggle-label,
  .mgr-toggle-row:has(input:checked) .mgr-toggle-label { color: var(--text); font-weight: 500; }

  /* Sticky footer */
  .mgr-footer {
    position: sticky; bottom: 16px;
    margin-top: 24px;
    background: var(--bg2);
    border: 1px solid rgba(251,191,36,0.3);
    border-radius: 14px;
    padding: 14px 18px;
    display: flex; justify-content: space-between; align-items: center;
    flex-wrap: wrap; gap: 12px;
    box-shadow: 0 -10px 30px -10px rgba(0,0,0,0.5);
    backdrop-filter: blur(10px);
  }
  /* Variant : modifications sauvegardées (générées + copiées) */
  .mgr-footer-saved {
    background: rgba(34, 197, 94, 0.08);
    border-color: rgba(126, 240, 192, 0.4);
  }

  /* Modals */
  .mgr-modal-backdrop {
    position: fixed; inset: 0;
    background: rgba(0,0,0,0.6);
    backdrop-filter: blur(8px);
    display: flex; align-items: center; justify-content: center;
    z-index: 1000;
    animation: modal-fade 0.2s ease-out;
  }
  @keyframes modal-fade { from { opacity: 0; } to { opacity: 1; } }
  .mgr-modal {
    background: var(--bg2);
    border: 1px solid var(--border-strong);
    border-radius: 16px;
    max-width: 520px; width: 90%;
    max-height: 85vh; overflow: hidden;
    display: flex; flex-direction: column;
    box-shadow: 0 30px 80px -20px rgba(0,0,0,0.8);
    animation: modal-slide 0.25s ease-out;
  }
  @keyframes modal-slide {
    from { transform: translateY(12px) scale(0.98); opacity: 0; }
    to   { transform: translateY(0) scale(1); opacity: 1; }
  }
  .mgr-modal-header {
    display: flex; justify-content: space-between; align-items: center;
    padding: 18px 22px;
    border-bottom: 1px solid var(--border);
  }
  .mgr-modal-header h3 { margin: 0; font-size: 16px; font-weight: 700; }
  .mgr-modal-body {
    padding: 22px;
    overflow-y: auto;
    display: flex; flex-direction: column; gap: 14px;
  }
  .mgr-modal-footer {
    padding: 14px 22px;
    border-top: 1px solid var(--border);
    display: flex; justify-content: flex-end; gap: 8px;
  }

  .mgr-emoji-grid {
    display: grid;
    grid-template-columns: repeat(9, 1fr);
    gap: 6px;
  }
  .mgr-emoji-btn {
    background: rgba(255,255,255,0.04);
    border: 1.5px solid transparent;
    color: var(--text);
    padding: 8px; font-size: 18px;
    border-radius: 8px; cursor: pointer;
    transition: all 0.15s ease;
  }
  .mgr-emoji-btn:hover { background: rgba(255,255,255,0.1); }
  .mgr-emoji-btn.active {
    background: rgba(126,240,192,0.1);
    border-color: #7ef0c0;
  }
  .mgr-color-grid {
    display: flex; flex-wrap: wrap; gap: 8px;
  }
  .mgr-color-btn {
    width: 32px; height: 32px;
    border-radius: 50%;
    border: 2px solid transparent;
    cursor: pointer;
    transition: all 0.15s ease;
  }
  .mgr-color-btn:hover { transform: scale(1.1); }
  .mgr-color-btn.active {
    border-color: #fff;
    box-shadow: 0 0 0 2px var(--bg);
  }

  .mgr-output {
    width: 100%;
    font-family: 'SF Mono', Monaco, monospace;
    font-size: 11px;
    background: rgba(0,0,0,0.3);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px;
    color: var(--text);
    resize: vertical;
  }

  .mgr-inline-link {
    background: rgba(126,240,192,0.12);
    border: 1px solid rgba(126,240,192,0.3);
    color: #7ef0c0;
    padding: 4px 10px;
    border-radius: 8px;
    font-size: 12px; font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
  }
  .mgr-inline-link:hover {
    background: rgba(126,240,192,0.22);
    transform: translateY(-1px);
  }
  @keyframes pulse-update {
    0%, 100% { box-shadow: inset 0 1px 0 rgba(255,255,255,0.25), 0 6px 18px -6px var(--accent-glow); }
    50%      { box-shadow: inset 0 1px 0 rgba(255,255,255,0.3),  0 10px 28px -4px var(--accent-glow); }
  }
  .log-prefix-fic { background: rgba(230, 57, 70, 0.18); color: var(--red-hot); }
  .fic-item {
    display: flex; align-items: center; gap: 10px;
    padding: 10px 0; border-bottom: 1px solid var(--border);
  }
  .fic-item:last-child { border-bottom: none; }
  .fic-main { flex: 1; display: flex; flex-direction: column; gap: 5px; min-width: 0; }
  .fic-name { font-size: 12px; color: var(--text); word-break: break-all; }
  .fic-meta { display: flex; flex-wrap: wrap; gap: 5px; }
  .fic-actions { display: flex; gap: 5px; flex: none; }

  /* Check Torrent */
  .check-tab { display: flex; flex-direction: column; gap: 14px; max-width: 1100px; }
  .check-header { display: flex; align-items: center; justify-content: space-between; }
  .check-actions { display: flex; gap: 8px; }
  .check-filters { display: flex; gap: 6px; }
  .filter-btn {
    display: inline-flex; align-items: center; gap: 7px;
    background: rgba(255,255,255,0.03);
    border: 1px solid var(--border);
    color: var(--text2);
    padding: 7px 14px; font-size: 12px; font-weight: 500;
    border-radius: 8px;
    text-transform: uppercase; letter-spacing: 0.4px;
    transition: all 160ms ease;
  }
  .filter-btn:hover {
    background: rgba(0, 180, 216, 0.06);
    border-color: rgba(0, 180, 216, 0.25);
    color: var(--text);
  }
  .filter-btn.active {
    background: rgba(0, 180, 216, 0.12);
    border-color: rgba(0, 180, 216, 0.4);
    color: var(--text);
  }
  .filter-count {
    background: rgba(0,0,0,0.3); color: var(--text2);
    padding: 1px 7px; border-radius: 9999px; font-size: 10px;
    font-variant-numeric: tabular-nums;
  }
  .filter-btn.active .filter-count { background: rgba(0, 180, 216, 0.25); color: var(--blue-hot); }

  .chk-card {
    background: linear-gradient(180deg, rgba(255,255,255,0.035) 0%, rgba(255,255,255,0.015) 100%);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 14px 16px;
    display: flex; flex-direction: column; gap: 8px;
    box-shadow: inset 0 1px 0 rgba(255,255,255,0.05);
  }
  .chk-card.err { border-color: rgba(239, 68, 68, 0.3); background: rgba(239, 68, 68, 0.04); }
  .chk-head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  .chk-name {
    font-weight: 600; color: var(--text); font-size: 13px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .chk-badge {
    font-size: 11px; font-weight: 600;
    padding: 3px 10px; border-radius: 9999px;
    background: rgba(255,255,255,0.06); color: var(--text2);
    border: 1px solid var(--border);
    flex: none;
    text-transform: uppercase; letter-spacing: 0.4px;
  }
  .chk-badge.err { background: rgba(239, 68, 68, 0.12); color: #ff9585; border-color: rgba(239, 68, 68, 0.3); }
  .chk-badge.ok  { background: rgba(34, 197, 94, 0.12); color: #7ef0c0; border-color: rgba(34, 197, 94, 0.3); }
  .chk-msg { font-size: 11px; color: #ff9585; }
  .chk-file { font-size: 11px; color: var(--text3); font-family: ui-monospace, Menlo, monospace; }
  .chk-lihdl { font-size: 12px; color: #7ef0c0; }
  .chk-lihdl code { font-family: ui-monospace, Menlo, monospace; font-size: 11px; background: rgba(0,0,0,0.25); padding: 1px 6px; border-radius: 4px; }
  .chk-nomatch { font-size: 11px; color: var(--text3); font-style: italic; }
  .chk-btn { margin-top: 4px; padding: 9px 18px; font-size: 13px; }
  .chk-progress { display: flex; align-items: center; gap: 8px; font-size: 11px; color: var(--text2); }
  .chk-progress .progress-bar { flex: 1; height: 7px; background: rgba(255,255,255,0.06); border-radius: 4px; overflow: hidden; }

  /* Barre de progression inline pour les actions Torrent→seedbox et Reseed complet */
  .check-prog {
    margin-top: -4px; margin-bottom: 12px; padding: 8px 12px;
    background: rgba(108, 99, 255, 0.08); border: 1px solid rgba(108, 99, 255, 0.2);
    border-left: 3px solid #6c63ff; border-radius: 0 6px 6px 0;
    font-size: 11px;
  }
  .check-prog.done { background: rgba(126, 240, 192, 0.08); border-color: rgba(126, 240, 192, 0.35); border-left-color: #7ef0c0; }
  .check-prog.err  { background: rgba(239, 68, 68, 0.08); border-color: rgba(239, 68, 68, 0.35); border-left-color: #ef4444; }
  .check-prog-bar  { height: 6px; background: rgba(255,255,255,0.06); border-radius: 3px; overflow: hidden; margin-bottom: 4px; }
  .check-prog-fill { height: 100%; background: linear-gradient(90deg, #6c63ff, #a78bfa); transition: width 0.25s; }
  .check-prog.done .check-prog-fill { background: linear-gradient(90deg, #22c55e, #7ef0c0); }
  .check-prog.err  .check-prog-fill { background: #ef4444; }
  .check-prog-meta { display: flex; justify-content: space-between; gap: 10px; align-items: center; color: var(--text2); }
  .check-prog-msg  { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .check-prog-stats { font-family: ui-monospace, monospace; font-variant-numeric: tabular-nums; color: var(--text); font-size: 10.5px; white-space: nowrap; }
  .chk-progress .progress-fill { height: 100%; background: var(--grad-primary); transition: width 0.15s; }
  .chk-pct { width: 40px; text-align: right; font-variant-numeric: tabular-nums; }
  .chk-status { min-width: 180px; }
  .chk-done { font-size: 12px; color: #7ef0c0; font-weight: 600; }
</style>
