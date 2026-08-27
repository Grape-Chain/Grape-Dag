/*
 * Drive the bundled wallet against a LIVE node.
 *
 * Unlike scripts/wallet_e2e.mjs (which uses a stub), this talks to a running
 * peer: it fetches the wallet's own assets over HTTP, runs the real signer,
 * pulls funds from the real faucet, and submits a real payment that the network
 * has to confirm in a pin.
 *
 * Usage:
 *   node scripts/wallet_live.mjs [baseUrl]        # default http://localhost:8010
 *
 * The node needs traffic for pins to form, so keep a generator running
 * alongside (txgen -mode trader) or expect confirmation to lag.
 */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';

const base = process.argv[2] || 'http://localhost:8010';
const here = path.dirname(fileURLToPath(import.meta.url));
const walletDir = path.join(here, '..', 'web', 'wallet');

let failures = 0;
const check = (name, ok, detail) => {
  if (ok) console.log('  ok   ' + name + (detail ? '  (' + detail + ')' : ''));
  else { failures++; console.error('  FAIL ' + name + (detail ? '\n       ' + detail : '')); }
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function api(p, options) {
  const res = await fetch(base + p, options);
  const text = await res.text();
  let body = null;
  if (text) { try { body = JSON.parse(text); } catch (_) { body = { raw: text }; } }
  return { status: res.status, ok: res.ok, body };
}

// ---------------------------------------------------------------- load signer

globalThis.performance = globalThis.performance ?? { now: () => Date.now() };
globalThis.fs = fs;
globalThis.process = process;
globalThis.require = () => fs;

const ready = new Promise((resolve) => { globalThis.onGrapeWalletReady = resolve; });
new Function(fs.readFileSync(path.join(walletDir, 'wasm_exec.js'), 'utf8'))();
const go = new globalThis.Go();
const { instance } = await WebAssembly.instantiate(
  fs.readFileSync(path.join(walletDir, 'wallet.wasm')), go.importObject);
go.run(instance);
await ready;
const signer = globalThis.grapeWallet;
const U = createRequire(import.meta.url)(path.join(walletDir, 'units.js'));

console.log('live wallet check against ' + base);

// ---------------------------------------------------------------- node is up

const info = await api('/api/rest/network-info');
check('node reachable', info.ok, 'chainId=' + JSON.stringify(info.body));
const pin0 = await api('/api/rest/pin-number');
check('pin height readable', pin0.ok, 'pin=' + JSON.stringify(pin0.body));

// the wallet's own assets come from the node
const idx = await fetch(base + '/wallet/');
const idxText = idx.ok ? await idx.text() : '';
check('node serves the wallet page', idx.ok && idxText.includes('Grape Wallet'));

// ---------------------------------------------------------------- new account

const me = signer.newWallet();
check('created an account', /^0x[0-9a-f]{40}$/.test(me.address), me.address);

const before = await api('/api/rest/accounts/' + me.address);
check('unknown account reads as zero', before.ok && U.formatUnits(before.body.balance) === '0',
  'balance=' + (before.body && before.body.balance));

// ---------------------------------------------------------------- faucet

const faucet = await api('/faucet?address=' + me.address);
check('faucet accepted the request', faucet.ok, 'status=' + faucet.status + ' ' + JSON.stringify(faucet.body).slice(0, 160));

let funded = 0n;
for (let i = 0; i < 40 && funded === 0n; i++) {
  await sleep(3000);
  const acct = await api('/api/rest/accounts/' + me.address);
  funded = U.toBigInt(acct.body && acct.body.balance);
}
check('faucet funds confirmed on chain', funded > 0n, 'balance=' + U.formatUnits(funded) + ' GRAPE');

if (funded === 0n) {
  console.log('\nno funds arrived - stopping before the payment step');
  console.log(failures + ' failure(s)');
  process.exit(1);
}

// ---------------------------------------------------------------- send

const recipient = signer.newWallet();
const amount = U.parseUnits('7.25');
const signed = signer.signPayment({
  privateKey: me.privateKey, to: recipient.address, amount, chainType: signer.defaultChainType,
});
check('signed a payment locally', !signed.error, signed.error || signed.hash);

const sent = await api('/api/rest/transactions', {
  method: 'POST',
  headers: { 'content-type': 'application/json' },
  body: JSON.stringify({ encodedTx: signed.encodedTx }),
});
check('node accepted the payment', sent.ok,
  'status=' + sent.status + ' ' + JSON.stringify(sent.body).slice(0, 200));

// ---------------------------------------------------------------- confirm

let credited = 0n;
for (let i = 0; i < 40 && credited === 0n; i++) {
  await sleep(3000);
  const acct = await api('/api/rest/accounts/' + recipient.address);
  credited = U.toBigInt(acct.body && acct.body.balance);
}
check('recipient credited', credited === U.toBigInt(amount),
  'got ' + U.formatUnits(credited) + ' want ' + U.formatUnits(amount));

const senderAfter = await api('/api/rest/accounts/' + me.address);
const remaining = U.toBigInt(senderAfter.body && senderAfter.body.balance);
check('sender debited', remaining === funded - U.toBigInt(amount),
  'remaining ' + U.formatUnits(remaining) + ', expected ' + U.formatUnits(funded - U.toBigInt(amount)));

// ---------------------------------------------------------------- history

const hist = await api('/api/rest/transactions?accounts=' + me.address + '&page=0&pageSize=25&sortOrder=DESC');
const txs = (hist.body && hist.body.transactions) || [];
check('history lists our transactions', hist.ok && txs.length >= 1, txs.length + ' tx');
const outgoing = txs.find((t) => (t.recipient || '').toLowerCase() === recipient.address.toLowerCase());
check('our payment appears in history', !!outgoing,
  outgoing ? U.formatUnits(outgoing.amount) + ' GRAPE to ' + U.shortAddr(outgoing.recipient) : 'not found');
if (outgoing) {
  check('history amount renders correctly', U.formatUnits(outgoing.amount) === U.formatUnits(amount),
    U.formatUnits(outgoing.amount));
}

// paging defaults must work with no query params at all (the nil-pointer fix)
const bare = await api('/api/rest/transactions');
check('GET /transactions with no params returns 200', bare.ok, 'status=' + bare.status);
const bareAcct = await api('/api/rest/accounts');
check('GET /accounts with no params returns 200', bareAcct.ok, 'status=' + bareAcct.status);

console.log(failures ? `\n${failures} failure(s)` : '\nlive wallet flow passed');
process.exit(failures ? 1 : 0);
