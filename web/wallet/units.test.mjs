/*
 * Unit tests for the wallet's value formatting.
 *
 * Run: node web/wallet/units.test.mjs
 *
 * These matter because the node's REST API is not uniform about radix: account
 * balances arrive as decimal strings while transaction amounts arrive as
 * 0x-hex. Getting that wrong shows the user a wildly wrong number.
 */
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const U = require('./units.js');

let failures = 0;
function check(name, got, want) {
  const ok = got === want;
  if (!ok) { failures++; console.error(`  FAIL ${name}\n    got  ${got}\n    want ${want}`); }
  else console.log(`  ok   ${name}`);
}
function checkThrows(name, fn) {
  try { fn(); failures++; console.error(`  FAIL ${name} (expected a throw)`); }
  catch (_) { console.log(`  ok   ${name}`); }
}

console.log('toBigInt');
check('decimal string', U.toBigInt('1000000000000000000'), 1000000000000000000n);
// a transaction amount as the API renders it: types.Hex.String() -> 0x + hex
check('hex string (1 GRAPE)', U.toBigInt('0x0de0b6b3a7640000'), 1000000000000000000n);
check('bare 0x', U.toBigInt('0x'), 0n);
check('empty', U.toBigInt(''), 0n);
check('null', U.toBigInt(null), 0n);
check('undefined', U.toBigInt(undefined), 0n);
check('garbage does not throw', U.toBigInt('not a number'), 0n);
check('whitespace', U.toBigInt('  42  '), 42n);

console.log('formatUnits');
check('one whole', U.formatUnits('1000000000000000000'), '1');
check('hex one whole', U.formatUnits('0x0de0b6b3a7640000'), '1');
check('half', U.formatUnits('500000000000000000'), '0.5');
check('zero', U.formatUnits('0'), '0');
check('grouping', U.formatUnits('1234567000000000000000000'), '1,234,567');
check('faucet grant (1000)', U.formatUnits('1000000000000000000000'), '1,000');
check('smallest unit truncates below 6dp', U.formatUnits('1'), '0');
check('six dp kept', U.formatUnits('1000000000000'), '0.000001');
check('trailing zeros trimmed', U.formatUnits('1500000000000000000'), '1.5');
check('negative', U.formatUnits('-2500000000000000000'), '-2.5');

console.log('parseUnits');
check('1', U.parseUnits('1'), '1000000000000000000');
check('1.5', U.parseUnits('1.5'), '1500000000000000000');
check('0.000000000000000001', U.parseUnits('0.000000000000000001'), '1');
check('leading dot', U.parseUnits('.5'), '500000000000000000');
check('trailing dot', U.parseUnits('5.'), '5000000000000000000');
check('big', U.parseUnits('1000000'), '1000000000000000000000000');
checkThrows('zero rejected', () => U.parseUnits('0'));
checkThrows('empty rejected', () => U.parseUnits(''));
checkThrows('lone dot rejected', () => U.parseUnits('.'));
checkThrows('letters rejected', () => U.parseUnits('1e18'));
checkThrows('negative rejected', () => U.parseUnits('-1'));
checkThrows('too many decimals rejected', () => U.parseUnits('0.0000000000000000001'));
checkThrows('comma rejected', () => U.parseUnits('1,5'));

console.log('round trip');
for (const v of ['1', '0.5', '123.456789', '1000000', '0.000000000000000001']) {
  const round = U.formatUnits(U.parseUnits(v), 18n, 18).replace(/,/g, '');
  check(`parse->format ${v}`, round, v.replace(/^\./, '0.').replace(/\.$/, ''));
}

console.log('shortAddr');
check('long address', U.shortAddr('0xdb63285447b5225943b9d83e913c46e7a9d21871'), '0xdb6328…d21871');
check('short passthrough', U.shortAddr('0x1234'), '0x1234');
check('empty', U.shortAddr(''), '');
check('null', U.shortAddr(null), '');

console.log(failures ? `\n${failures} failure(s)` : '\nall units tests passed');
process.exit(failures ? 1 : 0);
