export const wsDialTimeoutMs = 3_000;
export const wsMessageTimeoutMs = 10_000;
export const txSearchWindow = 20;
export const blockPageSize = 1;
export const sampleBlockPageSize = 3;
export const sampleBlockProbeMax = 3;
export const sciNotationWindow = 40;
export const sciNotationTxLimit = 8;
export const addressPage = 1;
export const addressPageSize = 10;
// Upper bound on transactions probed when resolving an (address, txid) pair whose txid is guaranteed
// to fall on the address's first page (see TestContext.getSampleAddressTx).
export const sampleAddrTxProbeMax = 40;
export const evmHistoryPage = 1;
export const evmHistoryPageSize = 3;
export const scientificNotationPattern = /"value(?:Zat|Sat)?"\s*:\s*-?\d+\.\d+[eE][+-]?\d+/;
