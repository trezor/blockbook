export const wsDialTimeoutMs = 3_000;
export const wsMessageTimeoutMs = 10_000;
export const txSearchWindow = 20;
export const blockPageSize = 1;
export const sampleBlockPageSize = 3;
// Low-activity chains (e.g. ethereum-classic, ~73% empty blocks) need a wide window to find a block
// that actually carries transactions; 3 blocks failed ~42% of the time.
export const sampleBlockProbeMax = txSearchWindow;
// Address sampling: the first sender of a random tx on a busy EVM chain can be a bot with 10^8 txs,
// which makes even a bounded balance history unservable. Prefer a participant below this tx count.
export const sampleAddressMaxTxs = 10_000;
// Probe budget: txs inspected and participants checked per tx (one details=basic request each).
export const sampleAddressProbeMax = 8;
export const sampleAddressCandidatesPerTx = 4;
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
