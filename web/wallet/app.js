'use strict';
/*
 * Grape testnet wallet.
 *
 * Signing happens in wallet.wasm, which is the node's own crypto/tx/wallet
 * packages compiled to WebAssembly - the browser therefore produces exactly the
 * bytes the ledger expects, and the private key never leaves this page.
 *
 * The key is held in memory while unlocked and stored in localStorage encrypted
 * with AES-GCM under a PBKDF2 key derived from the user's passphrase.
 */

// ---------------------------------------------------------------- constants

const API = '/api/rest';
const STORE_KEY = 'grape.wallet.v1';
const PBKDF2_ITERATIONS = 250000;
const POLL_MS = 5000;
const TX_PAGE_SIZE = 25;

// ---------------------------------------------------------------- state

let signer = null;      // the wasm API
let unlocked = null;    // {privateKey, publicKey, address} while unlocked
let pollTimer = null;
let pendingTx = null;   // payment awaiting confirmation

// ---------------------------------------------------------------- helpers

const $ = (id) => document.getElementById(id);

function show(el, on) { el.classList.toggle('hide', !on); }

function msg(el, text, kind) {
  el.textContent = text || '';
  el.className = 'msg' + (text ? ' show ' + (kind || 'info') : '');
}

const { toBigInt, formatUnits, parseUnits, shortAddr, fmtTime } = globalThis.GrapeUnits;

// ---------------------------------------------------------------- api

async function apiFetch(path, options = {}) {
  const res = await fetch(API + path, {
    credentials: 'same-origin',
    headers: { accept: 'application/json', ...(options.headers || {}) },
    ...options,
  });
  const text = await res.text();
  let body = null;
  if (text) { try { body = JSON.parse(text); } catch (_) { body = { error: text }; } }
  if (!res.ok) {
    const detail = (body && (body.error || body.message)) || res.statusText || ('HTTP ' + res.status);
    const err = new Error(detail);
    err.status = res.status;
    throw err;
  }
  return body;
}

const api = {
  account: (addr) => apiFetch('/accounts/' + encodeURIComponent(addr)),
  transactions: (addr) => apiFetch('/transactions?accounts=' + encodeURIComponent(addr)
    + '&page=0&pageSize=' + TX_PAGE_SIZE + '&sortOrder=DESC'),
  send: (encodedTx) => apiFetch('/transactions', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ encodedTx }),
  }),
  pinNumber: () => apiFetch('/pin-number'),
  networkInfo: () => apiFetch('/network-info'),
  peers: () => apiFetch('/peers'),
  faucet: (addr) => fetch('/faucet?address=' + encodeURIComponent(addr), { credentials: 'same-origin' })
    .then(async (res) => {
      const text = await res.text();
      let body = null;
      if (text) { try { body = JSON.parse(text); } catch (_) { body = { error: text }; } }
      if (!res.ok) throw new Error((body && body.error) || ('HTTP ' + res.status));
      return body;
    }),
};

// ---------------------------------------------------------------- key storage

function b64(bytes) { return btoa(String.fromCharCode(...new Uint8Array(bytes))); }
function unb64(s) { return Uint8Array.from(atob(s), (c) => c.charCodeAt(0)); }

async function deriveKey(passphrase, salt) {
  const material = await crypto.subtle.importKey('raw', new TextEncoder().encode(passphrase),
    'PBKDF2', false, ['deriveKey']);
  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt, iterations: PBKDF2_ITERATIONS, hash: 'SHA-256' },
    material, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt']);
}

async function storeWallet(wallet, passphrase) {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const key = await deriveKey(passphrase, salt);
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key,
    new TextEncoder().encode(wallet.privateKey));
  localStorage.setItem(STORE_KEY, JSON.stringify({
    v: 1, address: wallet.address, salt: b64(salt), iv: b64(iv), key: b64(ct),
    iterations: PBKDF2_ITERATIONS,
  }));
}

function storedWallet() {
  try { return JSON.parse(localStorage.getItem(STORE_KEY) || 'null'); } catch (_) { return null; }
}

async function decryptWallet(record, passphrase) {
  const key = await deriveKey(passphrase, unb64(record.salt));
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: unb64(record.iv) }, key, unb64(record.key));
  return new TextDecoder().decode(pt);
}

// ---------------------------------------------------------------- wasm

function loadSigner() {
  return new Promise((resolve, reject) => {
    globalThis.onGrapeWalletReady = () => resolve(globalThis.grapeWallet);
    const go = new globalThis.Go();
    WebAssembly.instantiateStreaming(fetch('wallet.wasm'), go.importObject)
      .then((res) => { go.run(res.instance); })
      .catch(reject);
    setTimeout(() => reject(new Error('the signer did not start')), 60000);
  });
}

// ---------------------------------------------------------------- views

function showSetup() {
  show($('setup'), true); show($('locked'), false); show($('app'), false); show($('lockBtn'), false);
  stopPolling();
}

function showLocked(record) {
  show($('setup'), false); show($('locked'), true); show($('app'), false); show($('lockBtn'), false);
  $('lockedAddr').textContent = 'Wallet ' + shortAddr(record.address);
  stopPolling();
}

function showApp() {
  show($('setup'), false); show($('locked'), false); show($('app'), true); show($('lockBtn'), true);
  $('myAddr').textContent = unlocked.address;
  refreshAll();
  startPolling();
}

function route() {
  const record = storedWallet();
  if (!record) return showSetup();
  if (!unlocked) return showLocked(record);
  return showApp();
}

// ---------------------------------------------------------------- data refresh

async function refreshAccount() {
  try {
    const acct = await api.account(unlocked.address);
    $('balance').textContent = formatUnits(acct && acct.balance);
    $('acctNonce').textContent = acct && acct.nonce !== undefined ? acct.nonce : '0';
    msg($('acctMsg'), '');
  } catch (e) {
    if (e.status === 401) return authFailed();
    $('balance').textContent = '0';
    msg($('acctMsg'), 'Could not read the account: ' + e.message, 'err');
  }
}

async function refreshNode() {
  try {
    const [pin, info] = await Promise.all([
      api.pinNumber().catch(() => null),
      api.networkInfo().catch(() => null),
    ]);
    $('nodePin').textContent = pin && (pin.pinNumber ?? pin.pinTxNumber ?? pin.number) !== undefined
      ? (pin.pinNumber ?? pin.pinTxNumber ?? pin.number) : '—';
    const chain = info && (info.chainId || info.chainID);
    $('nodeChain').textContent = chain || '—';
    if (chain) $('netLabel').textContent = String(chain).toLowerCase().includes('main') ? 'mainnet' : 'testnet';
    $('nodeStatus').textContent = 'connected';
  } catch (e) {
    $('nodeStatus').textContent = 'unreachable';
  }
  try {
    const peers = await api.peers();
    const list = (peers && (peers.data || peers.peers)) || [];
    $('nodePeers').textContent = Array.isArray(list) ? list.length : '—';
  } catch (_) {
    $('nodePeers').textContent = '—';
  }
}

async function refreshTxs() {
  try {
    const res = await api.transactions(unlocked.address);
    const txs = (res && res.transactions) || [];
    const body = $('txBody');
    body.textContent = '';
    if (!txs.length) {
      show($('txTable'), false); show($('txEmpty'), true);
      return;
    }
    const me = unlocked.address.toLowerCase();
    for (const tx of txs) {
      const outgoing = (tx.sender || '').toLowerCase() === me;
      const counterparty = outgoing ? tx.recipient : tx.sender;
      const confirmed = tx.pinTxNumber !== undefined && tx.pinTxNumber !== null && tx.pinTxNumber >= 0;
      const tr = document.createElement('tr');

      const dirCell = document.createElement('td');
      const pill = document.createElement('span');
      pill.className = 'pill ' + (confirmed ? (outgoing ? 'out' : 'in') : 'pending');
      pill.textContent = confirmed ? (outgoing ? 'sent' : 'received') : 'pending';
      dirCell.appendChild(pill);

      const partyCell = document.createElement('td');
      partyCell.className = 'mono';
      partyCell.textContent = shortAddr(counterparty);
      partyCell.title = counterparty || '';

      const whenCell = document.createElement('td');
      whenCell.textContent = fmtTime(tx.timestamp);

      const amtCell = document.createElement('td');
      amtCell.className = 'num';
      amtCell.textContent = (outgoing ? '-' : '+') + formatUnits(tx.amount);

      const pinCell = document.createElement('td');
      pinCell.className = 'num';
      pinCell.textContent = confirmed ? tx.pinTxNumber : '—';

      tr.append(dirCell, partyCell, whenCell, amtCell, pinCell);
      body.appendChild(tr);
    }
    show($('txTable'), true); show($('txEmpty'), false);
    msg($('txMsg'), '');
  } catch (e) {
    if (e.status === 401) return authFailed();
    msg($('txMsg'), 'Could not load transactions: ' + e.message, 'err');
  }
}

function refreshAll() {
  if (!unlocked) return;
  refreshAccount(); refreshNode(); refreshTxs();
}

function startPolling() {
  stopPolling();
  pollTimer = setInterval(() => { refreshAccount(); refreshNode(); refreshTxs(); }, POLL_MS);
}
function stopPolling() { if (pollTimer) { clearInterval(pollTimer); pollTimer = null; } }

function authFailed() {
  stopPolling();
  msg($('acctMsg'), 'The node rejected the request (not authorised). Reload the page and sign in, '
    + 'or run the node with peer.apiauthdisabled for local development.', 'err');
}

// ---------------------------------------------------------------- actions

async function createWallet() {
  const p1 = $('newPass').value, p2 = $('newPass2').value;
  if (p1.length < 8) return msg($('setupMsg'), 'Use a passphrase of at least 8 characters.', 'err');
  if (p1 !== p2) return msg($('setupMsg'), 'The passphrases do not match.', 'err');

  const created = signer.newWallet();
  if (created.error) return msg($('setupMsg'), created.error, 'err');
  await storeWallet(created, p1);
  unlocked = created;
  $('newPass').value = $('newPass2').value = '';
  msg($('setupMsg'), '');
  route();
}

async function importWallet() {
  const p1 = $('newPass').value, p2 = $('newPass2').value;
  if (p1.length < 8) return msg($('setupMsg'), 'Use a passphrase of at least 8 characters.', 'err');
  if (p1 !== p2) return msg($('setupMsg'), 'The passphrases do not match.', 'err');
  const raw = $('importKey').value.trim();
  if (!raw) return msg($('setupMsg'), 'Paste the private key to import.', 'err');

  const imported = signer.importPrivateKey(raw);
  if (imported.error) return msg($('setupMsg'), imported.error, 'err');
  await storeWallet(imported, p1);
  unlocked = imported;
  $('importKey').value = '';
  $('newPass').value = $('newPass2').value = '';
  msg($('setupMsg'), '');
  route();
}

async function unlock() {
  const record = storedWallet();
  const pass = $('unlockPass').value;
  if (!pass) return msg($('lockedMsg'), 'Enter your passphrase.', 'err');
  msg($('lockedMsg'), 'Unlocking…', 'info');
  let priv;
  try {
    priv = await decryptWallet(record, pass);
  } catch (_) {
    return msg($('lockedMsg'), 'That passphrase does not match this wallet.', 'err');
  }
  const w = signer.importPrivateKey(priv);
  if (w.error) return msg($('lockedMsg'), w.error, 'err');
  if (record.address && w.address !== record.address) {
    return msg($('lockedMsg'), 'The stored key does not match the stored address; refusing to continue.', 'err');
  }
  unlocked = w;
  $('unlockPass').value = '';
  msg($('lockedMsg'), '');
  route();
}

function lock() {
  unlocked = null;
  stopPolling();
  route();
}

function forget() {
  if (!confirm('Remove this wallet from the browser? Anything not backed up is lost for good.')) return;
  localStorage.removeItem(STORE_KEY);
  unlocked = null;
  route();
}

async function faucet() {
  msg($('acctMsg'), 'Requesting testnet GRAPE…', 'info');
  $('faucetBtn').disabled = true;
  try {
    await api.faucet(unlocked.address);
    msg($('acctMsg'), 'Faucet payment submitted. The balance updates once it is confirmed in a pin.', 'ok');
    setTimeout(refreshAll, 1500);
  } catch (e) {
    msg($('acctMsg'), 'Faucet: ' + e.message, 'err');
  } finally {
    $('faucetBtn').disabled = false;
  }
}

function reviewSend() {
  const to = $('toAddr').value.trim();
  let amount;
  try {
    amount = parseUnits($('amount').value);
  } catch (e) {
    return msg($('sendMsg'), e.message, 'err');
  }
  const check = signer.validateAddress(to);
  if (check.error || !check.valid) return msg($('sendMsg'), 'That recipient address is not valid.', 'err');
  if (to.toLowerCase() === unlocked.address.toLowerCase()) {
    return msg($('sendMsg'), 'That is this wallet’s own address.', 'err');
  }
  msg($('sendMsg'), '');
  pendingTx = { to, amount };
  $('cTo').textContent = to;
  $('cAmount').textContent = formatUnits(amount) + ' GRAPE';
  $('confirmDlg').showModal();
}

async function confirmSend() {
  const { to, amount } = pendingTx || {};
  $('confirmDlg').close();
  if (!to) return;
  msg($('sendMsg'), 'Signing…', 'info');
  $('sendBtn').disabled = true;
  try {
    const signed = signer.signPayment({
      privateKey: unlocked.privateKey,
      to,
      amount,
      chainType: signer.defaultChainType,
    });
    if (signed.error) throw new Error(signed.error);
    msg($('sendMsg'), 'Sending…', 'info');
    await api.send(signed.encodedTx);
    msg($('sendMsg'), 'Sent. Transaction ' + shortAddr(signed.hash) + ' is now in the pool.', 'ok');
    $('toAddr').value = ''; $('amount').value = '';
    setTimeout(refreshAll, 1200);
  } catch (e) {
    msg($('sendMsg'), 'Could not send: ' + e.message, 'err');
  } finally {
    $('sendBtn').disabled = false;
    pendingTx = null;
  }
}

// ---------------------------------------------------------------- wiring

function wire() {
  $('createBtn').onclick = createWallet;
  $('showImportBtn').onclick = () => {
    const area = $('importArea');
    area.classList.toggle('hide');
  };
  $('importBtn').onclick = importWallet;
  $('unlockBtn').onclick = unlock;
  $('forgetBtn').onclick = forget;
  $('lockBtn').onclick = lock;
  $('refreshBtn').onclick = refreshAll;
  $('faucetBtn').onclick = faucet;
  $('sendBtn').onclick = reviewSend;
  $('cCancel').onclick = () => { $('confirmDlg').close(); pendingTx = null; };
  $('cConfirm').onclick = confirmSend;
  $('eClose').onclick = () => $('exportDlg').close();

  $('copyAddrBtn').onclick = async () => {
    try {
      await navigator.clipboard.writeText(unlocked.address);
      msg($('acctMsg'), 'Address copied.', 'ok');
    } catch (_) {
      msg($('acctMsg'), 'Could not copy - select the address and copy it manually.', 'err');
    }
  };

  $('exportBtn').onclick = () => {
    $('exportKeyText').textContent = unlocked.privateKey;
    $('exportDlg').showModal();
  };

  for (const id of ['newPass2', 'importKey']) {
    $(id).addEventListener('keydown', (e) => {
      if (e.key === 'Enter') (id === 'importKey' ? $('importBtn') : $('createBtn')).click();
    });
  }
  $('unlockPass').addEventListener('keydown', (e) => { if (e.key === 'Enter') $('unlockBtn').click(); });
  $('amount').addEventListener('keydown', (e) => { if (e.key === 'Enter') $('sendBtn').click(); });

  document.addEventListener('visibilitychange', () => {
    if (document.hidden) stopPolling();
    else if (unlocked) { refreshAll(); startPolling(); }
  });
}

async function main() {
  wire();
  if (!crypto || !crypto.subtle) {
    $('wasmStatus').textContent = 'this browser cannot store keys securely';
    return;
  }
  try {
    signer = await loadSigner();
    $('wasmStatus').textContent = 'signer ready';
    setTimeout(() => { $('wasmStatus').textContent = ''; }, 2500);
  } catch (e) {
    $('wasmStatus').textContent = 'signer failed to load';
    msg($('setupMsg'), 'The signer (wallet.wasm) could not be loaded: ' + e.message
      + '. If you are running a node from source, build it with "make wallet".', 'err');
    show($('setup'), true);
    return;
  }
  route();
}

main();
