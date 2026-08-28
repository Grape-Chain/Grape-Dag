'use strict';
/*
 * Value formatting for the Grape wallet.
 *
 * Kept separate from app.js so it can be unit-tested outside a browser: this is
 * the code that turns ledger integers into what the user reads, and a rounding
 * or radix mistake here is a wrong amount on screen or in a signed transaction.
 *
 * Note the API is not uniform about radix - account balances arrive as decimal
 * strings (big.Int.Text(10)) while transaction amounts arrive 0x-hex-encoded
 * (types.Hex.String()) - so every value is parsed tolerantly.
 */
(function (root) {
  const DECIMALS = 18n;

  /** Parse an integer the API may render as decimal or as 0x-hex. */
  function toBigInt(value) {
    if (value === null || value === undefined || value === '') return 0n;
    const s = String(value).trim();
    if (s === '' || s === '0x' || s === '0X') return 0n;
    try {
      if (s.startsWith('0x') || s.startsWith('0X')) return BigInt(s);
      return BigInt(s);
    } catch (_) {
      return 0n;
    }
  }

  /** Render a base-unit integer as a decimal string, grouped, zeros trimmed. */
  function formatUnits(raw, decimals = DECIMALS, maxFractionDigits = 6) {
    let v = toBigInt(raw);
    const neg = v < 0n;
    if (neg) v = -v;
    const base = 10n ** BigInt(decimals);
    const whole = (v / base).toString();
    // truncate to the digits we are willing to show FIRST, then drop trailing
    // zeros - the other order renders one base unit as "0.000000", which reads
    // as a precise zero rather than "smaller than we display"
    const frac = (v % base).toString().padStart(Number(decimals), '0')
      .slice(0, maxFractionDigits)
      .replace(/0+$/, '');
    const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    return (neg ? '-' : '') + grouped + (frac ? '.' + frac : '');
  }

  /** Decimal string -> base units, as a decimal string. Throws on bad input. */
  function parseUnits(text, decimals = DECIMALS) {
    const s = String(text).trim();
    if (s === '' || s === '.' || !/^\d*\.?\d*$/.test(s)) {
      throw new Error('Enter a number, for example 1.5');
    }
    const [whole, frac = ''] = s.split('.');
    if (frac.length > Number(decimals)) {
      throw new Error('Too many decimal places (max ' + decimals + ')');
    }
    const v = BigInt((whole || '0') + frac.padEnd(Number(decimals), '0'));
    if (v <= 0n) throw new Error('Amount must be greater than zero');
    return v.toString();
  }

  function shortAddr(a) {
    if (!a || a.length < 12) return a || '';
    return a.slice(0, 8) + '…' + a.slice(-6);
  }

  function fmtTime(ts) {
    if (!ts) return '';
    const s = String(ts);
    const d = new Date(s.endsWith('Z') || s.includes('+') ? s : s + 'Z');
    if (isNaN(d.getTime())) return s;
    return d.toLocaleString();
  }

  const api = { DECIMALS, toBigInt, formatUnits, parseUnits, shortAddr, fmtTime };

  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  root.GrapeUnits = api;
})(typeof globalThis !== 'undefined' ? globalThis : this);
