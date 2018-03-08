package bchain

type ScriptSig struct {
	// Asm string `json:"asm"`
	Hex string `json:"hex"`
}

type Vin struct {
	Coinbase  string    `json:"coinbase"`
	Txid      string    `json:"txid"`
	Vout      uint32    `json:"vout"`
	ScriptSig ScriptSig `json:"scriptSig"`
	Sequence  uint32    `json:"sequence"`
}

type ScriptPubKey struct {
	Asm       string   `json:"asm"`
	Hex       string   `json:"hex,omitempty"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
}

type Vout struct {
	Value        float64      `json:"value"`
	N            uint32       `json:"n"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
}

// Tx is blockchain transaction
// unnecessary fields are commented out to avoid overhead
type Tx struct {
	Hex  string `json:"hex"`
	Txid string `json:"txid"`
	// Version  int32  `json:"version"`
	LockTime uint32 `json:"locktime"`
	Vin      []Vin  `json:"vin"`
	Vout     []Vout `json:"vout"`
	// BlockHash     string `json:"blockhash,omitempty"`
	Confirmations uint32 `json:"confirmations,omitempty"`
	Time          int64  `json:"time,omitempty"`
	Blocktime     int64  `json:"blocktime,omitempty"`
}

type Block struct {
	BlockHeader
	Txs []Tx `json:"tx"`
}

type ThinBlock struct {
	BlockHeader
	Txids []string `json:"tx"`
}

type BlockHeader struct {
	Hash          string `json:"hash"`
	Prev          string `json:"previousblockhash"`
	Next          string `json:"nextblockhash"`
	Height        uint32 `json:"height"`
	Confirmations int    `json:"confirmations"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type BlockChain interface {
	// chain info
	IsTestnet() bool
	GetNetworkName() string
	// requests
	GetBestBlockHash() (string, error)
	GetBestBlockHeight() (uint32, error)
	GetBlockHash(height uint32) (string, error)
	GetBlockHeader(hash string) (*BlockHeader, error)
	GetBlock(hash string) (*Block, error)
	GetBlockWithoutHeader(hash string, height uint32) (*Block, error)
	GetMempool() ([]string, error)
	GetTransaction(txid string) (*Tx, error)
	EstimateSmartFee(blocks int, conservative bool) (float64, error)
	SendRawTransaction(tx string) (string, error)
	// mempool
	ResyncMempool(onNewTxAddr func(txid string, addr string)) error
	GetMempoolTransactions(outputScript []byte) ([]string, error)
	GetMempoolSpentOutput(outputTxid string, vout uint32) string
	// parser
	GetChainParser() BlockChainParser
}

type BlockChainParser interface {
	AddressToOutputScript(address string) ([]byte, error)
	OutputScriptToAddresses(script []byte) ([]string, error)
	ParseTx(b []byte) (*Tx, error)
	ParseBlock(b []byte) (*Block, error)
	PackTx(tx *Tx, height uint32, blockTime int64) ([]byte, error)
	UnpackTx(buf []byte) (*Tx, uint32, error)
}
