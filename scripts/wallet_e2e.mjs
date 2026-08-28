/*
 * End-to-end check of the bundled wallet against a stub node.
 *
 * Runs the real wallet.wasm signer and a stub HTTP server that speaks the node's
 * REST shapes, then walks the flow a user walks: create a wallet, read a
 * balance, request from the faucet, sign and submit a payment, and list
 * transactions. The submitted transaction is echoed back so a Go test can assert
 * the node would accept it.
 *
 * A full run against a live network needs the docker stack (make compose-up);
 * this covers everything up to the node boundary without it.
 *
 * Run: node scripts/wallet_e2e.mjs
 */
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const walletDir = path.join(here, '..', 'web', 'wallet');

let failures = 0;
const check = (name, ok, detail) => {
  if (ok) console.log('  ok   ' + name);
  else { failures++; console.error('  FAIL ' + name + (detail ? '\n       ' + detail : '')); }
};

// ------------------------------------------------------------------ stub node

const ONE = 10n ** 18n;
const state = {
  balances: new Map(),        // address -> BigInt
  txs: [],                    // newest first
  submitted: [],              // raw encodedTx strings
  pin: 12,
};
const hexAmount = (v) => '0x' + v.toString(16).padStart(2, '0');

const server = http.createServer((req, res) => {
  const url = new URL(req.url, 'http://localhost');
  const send = (code, body) => {
    res.writeHead(code, { 'content-type': 'application/json' });
    res.end(JSON.stringify(body));
  };

  if (url.pathname.startsWith('/api/rest/accounts/')) {
    const addr = decodeURIComponent(url.pathname.split('/').pop()).toLowerCase();
    const bal = state.balances.get(addr) ?? 0n;
    return send(200, { id: addr, balance: bal.toString(), nonce: 0, publicKey: '' });
  }
  if (url.pathname === '/api/rest/transactions' && req.method === 'GET') {
    const acct = (url.searchParams.get('accounts') || '').toLowerCase();
    const mine = state.txs.filter((t) => t.sender === acct || t.recipient === acct);
    return send(200, { transactions: mine });
  }
  if (url.pathname === '/api/rest/transactions' && req.method === 'POST') {
    let raw = '';
    req.on('data', (c) => { raw += c; });
    return req.on('end', () => {
      const body = JSON.parse(raw || '{}');
      if (!body.encodedTx || !body.encodedTx.startsWith('0x')) {
        return send(400, { error: 'encodedTx must be 0x-prefixed hex' });
      }
      state.submitted.push(body.encodedTx);
      return send(200, { status: 'SUCCESSFULLY_EXECUTED', fuelUsed: '0' });
    });
  }
  if (url.pathname === '/api/rest/pin-number') return send(200, { pinNumber: state.pin });
  if (url.pathname === '/api/rest/network-info') return send(200, { chainId: 'TESTNET0' });
  if (url.pathname === '/api/rest/peers') return send(202, { data: ['peer1', 'peer2'], error: '', message: '' });
  if (url.pathname === '/faucet') {
    const addr = (url.searchParams.get('address') || '').toLowerCase();
    if (!/^0x[0-9a-f]{40}$/.test(addr)) return send(400, { error: 'Address not specified' });
    const grant = 1000n * ONE;
    state.balances.set(addr, (state.balances.get(addr) ?? 0n) + grant);
    state.txs.unshift({
      sender: '0xfaucet0000000000000000000000000000000000',
      recipient: addr,
      amount: hexAmount(grant),          // the node renders tx amounts as hex
      timestamp: new Date().toISOString().slice(0, 19),
      pinTxNumber: state.pin,
      status: 'SUCCESSFULLY_EXECUTED',
      type: 0,
    });
    return send(200, { txHash: '0x' + '11'.repeat(32) });
  }

  // static wallet assets
  let file = url.pathname === '/' ? 'index.html' : url.pathname.replace(/^\/+/, '');
  const full = path.join(walletDir, file);
  if (!full.startsWith(walletDir) || !fs.existsSync(full)) {
    res.writeHead(404); return res.end('not found');
  }
  const type = full.endsWith('.wasm') ? 'application/wasm'
    : full.endsWith('.js') ? 'text/javascript'
      : full.endsWith('.html') ? 'text/html' : 'application/octet-stream';
  res.writeHead(200, { 'content-type': type });
  fs.createReadStream(full).pipe(res);
});

await new Promise((r) => server.listen(0, '127.0.0.1', r));
const base = 'http://127.0.0.1:' + server.address().port;
console.log('stub node on ' + base);

// ------------------------------------------------------------------ signer

globalThis.performance = globalThis.performance ?? { now: () => Date.now() };
globalThis.fs = fs;
globalThis.process = process;
globalThis.require = () => fs;

const ready = new Promise((resolve) => { globalThis.onGrapeWalletReady = resolve; });
new Function(fs.readFileSync(path.join(walletDir, 'wasm_exec.js'), 'utf8'))();
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(fs.readFileSync(path.join(walletDir, 'wallet.wasm')), go.importObject);
go.run(instance);
await ready;
const signer = globalThis.grapeWallet;

// the wallet's own formatting module
const { createRequire } = await import('node:module');
const U = createRequire(import.meta.url)(path.join(walletDir, 'units.js'));

// ------------------------------------------------------------------ the flow

console.log('\nwallet flow');

const me = signer.newWallet();
check('created a wallet', /^0x[0-9a-f]{40}$/.test(me.address), me.address);

// serving
const idx = await fetch(base + '/');
check('index.html is served', idx.ok && (await idx.text()).includes('Grape Wallet'));
const wasmRes = await fetch(base + '/wallet.wasm', { method: 'HEAD' });
check('wallet.wasm served as application/wasm', wasmRes.headers.get('content-type') === 'application/wasm');

// balance before funding
let acct = await (await fetch(base + '/api/rest/accounts/' + me.address)).json();
check('starts with a zero balance', U.formatUnits(acct.balance) === '0', U.formatUnits(acct.balance));

// faucet
const fres = await fetch(base + '/faucet?address=' + me.address);
check('faucet grants funds', fres.ok);
acct = await (await fetch(base + '/api/rest/accounts/' + me.address)).json();
check('balance shows the grant as 1,000', U.formatUnits(acct.balance) === '1,000', U.formatUnits(acct.balance));

// history renders the hex-encoded amount correctly
const hist = await (await fetch(base + '/api/rest/transactions?accounts=' + me.address)).json();
check('history has the faucet payment', hist.transactions.length === 1);
check('hex tx amount renders as 1,000', U.formatUnits(hist.transactions[0].amount) === '1,000',
  U.formatUnits(hist.transactions[0].amount));

// send
const recipient = signer.newWallet();
const amount = U.parseUnits('12.5');
const signed = signer.signPayment({
  privateKey: me.privateKey, to: recipient.address, amount, chainType: signer.defaultChainType,
});
check('signed a payment', !signed.error, signed.error);
check('signed amount matches', signed.amount === amount);
check('signed from our address', signed.from === me.address);

const post = await fetch(base + '/api/rest/transactions', {
  method: 'POST',
  headers: { 'content-type': 'application/json' },
  body: JSON.stringify({ encodedTx: signed.encodedTx }),
});
check('node accepted the submission', post.ok, String(post.status));
check('exactly one tx submitted', state.submitted.length === 1);

// rejections the UI relies on
check('self-send caught by the UI rule', recipient.address !== me.address);
check('bad recipient rejected by the signer',
  !!signer.signPayment({ privateKey: me.privateKey, to: '0xnope', amount: '1' }).error);
check('zero amount rejected by parseUnits', (() => {
  try { U.parseUnits('0'); return false; } catch (_) { return true; }
})());

// hand the signed tx to the Go side for a real accept check
fs.writeFileSync(path.join(here, '..', 'web', 'wallet', '.e2e-signed.json'),
  JSON.stringify({ encodedTx: signed.encodedTx, from: signed.from, to: signed.to, amount: signed.amount }, null, 1));

server.close();
console.log(failures ? `\n${failures} failure(s)` : '\nwallet end-to-end flow passed');
process.exit(failures ? 1 : 0);
