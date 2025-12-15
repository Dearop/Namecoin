export function isValidDomain(domain) {
  const domainRegex = /^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]?\.[a-zA-Z]{2,}$/;
  return domainRegex.test(domain);
}

export function isValidFee(fee) {
  return typeof fee === 'number' && fee > 0 && Number.isInteger(fee);
}

export function isValidHex(str) {
  return /^[0-9a-fA-F]+$/.test(str);
}

export function isValidWalletID(id) {
  return id && typeof id === 'string' && isValidHex(id) && id.length >= 32;
}
