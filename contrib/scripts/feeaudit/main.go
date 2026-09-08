// usr/bin/go run $0 $@ ; exit

// feeaudit compares the EIP-1559 fees a public Blockbook quotes against what
// transactions on that chain actually paid.
//
// Blockbook serves EVM fee tiers from one of two sources: an alternative provider
// (Infura Gas API, returned verbatim including its own padding) or an on-chain
// estimate derived from eth_feeHistory. Nothing today measures either against
// reality, so this walks the whole path from public endpoints only:
//
//	quote      - websocket estimateFee, anchored to a block height via subscribeNewBlock
//	reality    - /api/v2/block/<h>, which carries every tx's effectiveGasPrice and gasUsed
//	counterfactual - eth_feeHistory reconstructed from that same block data, so the
//	                 provider's numbers can be A/B'd against our own algorithm with no
//	                 API key and no backend access
//
// Usage:
//
//	go run contrib/scripts/feeaudit/main.go -chains eth,bsc -duration 30m -out /tmp/feeaudit
//	go run contrib/scripts/feeaudit/main.go -chains eth -backfill 200
//
// With a 1inch key it also polls api.1inch.dev under identical market conditions,
// so the served provider, 1inch and the on-chain algorithm are compared side by
// side rather than across time:
//
//	go run contrib/scripts/feeaudit/main.go -chains eth,pol -duration 30m -oneinch-key-file ~/.config/1inch.key
//
// The counterfactual is only worth anything if the reconstruction is right, so
// check it against a real node before trusting a run (verified against coreth and
// op-reth, wei-for-wei):
//
//	go run contrib/scripts/feeaudit/main.go -chains avax -validate https://api.avax.network/ext/bc/C/rpc
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Public Blockbook hosts. Every entry except bsc is configured with the Infura
// alternative fee provider; bsc has it disabled ("infura-disabled"), which makes
// it the control group for the on-chain path.
var defaultChains = []string{"eth", "bsc", "pol", "arb", "op", "base", "avax", "hype", "rhc"}

func host(chain string) string { return chain + ".trezor.io" }

// suiteClamp mirrors EVM_GAS_PRICE_PER_CHAIN_IN_GWEI in trezor-suite
// packages/connect/src/data/defaultFeeLevels.ts. Suite applies these to whatever
// Blockbook returns, so a tier can reach the user materially changed.
type suiteClamp struct{ minGwei, maxGwei, minPriorityGwei float64 }

var suiteClamps = map[string]suiteClamp{
	"eth":  {0.001, 10000, 0},
	"pol":  {0.1, 10000000, 30},
	"bsc":  {0.001, 100000, 0},
	"base": {0.0000001, 1000, 0},
	"arb":  {0.001, 1000, 0},
	"op":   {0.000000001, 1000, 0},
	"rhc":  {0.001, 1000, 0},
	"hype": {0.001, 1000, 0},
}

// defaultClamp is connect's fallback for chains absent from the table above.
var defaultClamp = suiteClamp{0.000000001, 10000, 0}

func clampFor(chain string) suiteClamp {
	if c, ok := suiteClamps[chain]; ok {
		return c
	}
	return defaultClamp
}

// oneInchChainIDs are the EVM chain ids behind api.1inch.dev/gas-price/v1.5/<id>.
// Chains absent here (and chains 1inch does not cover) are simply not compared.
var oneInchChainIDs = map[string]int{
	"eth": 1, "bsc": 56, "pol": 137, "arb": 42161,
	"op": 10, "base": 8453, "avax": 43114,
}

// feeHistoryPercentiles are the reward percentiles Blockbook's on-chain path asks
// for, in tier order (ethrpc.go: EthereumTypeGetEip1559Fees).
var feeHistoryPercentiles = []float64{20, 70, 90, 99}

var tierNames = []string{"low", "medium", "high", "instant"}

// eip1559BaseFeeMultiplier mirrors the constant of the same name in ethrpc.go.
const eip1559BaseFeeMultiplier = 2

// feeHistoryBlocks is the window eth_feeHistory is called with on the on-chain path.
const feeHistoryBlocks = 4

const gwei = 1e9

// ---------------------------------------------------------------- wire types

// Local mirrors of server/ws_types.go and api/types.go. They are not imported:
// the server package pulls in RocksDB via cgo, which a standalone probe must not
// need. Field names and JSON tags are kept identical so drift is easy to spot.

type eip1559Fee struct {
	MaxFeePerGas         string `json:"maxFeePerGas"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	MinWaitTimeEstimate  int    `json:"minWaitTimeEstimate,omitempty"`
	MaxWaitTimeEstimate  int    `json:"maxWaitTimeEstimate,omitempty"`
}

type eip1559Fees struct {
	BaseFeePerGas              string      `json:"baseFeePerGas,omitempty"`
	Low                        *eip1559Fee `json:"low,omitempty"`
	Medium                     *eip1559Fee `json:"medium,omitempty"`
	High                       *eip1559Fee `json:"high,omitempty"`
	Instant                    *eip1559Fee `json:"instant,omitempty"`
	NetworkCongestion          float64     `json:"networkCongestion,omitempty"`
	LatestPriorityFeeRange     []string    `json:"latestPriorityFeeRange,omitempty"`
	HistoricalPriorityFeeRange []string    `json:"historicalPriorityFeeRange,omitempty"`
	HistoricalBaseFeeRange     []string    `json:"historicalBaseFeeRange,omitempty"`
	PriorityFeeTrend           string      `json:"priorityFeeTrend,omitempty"`
	BaseFeeTrend               string      `json:"baseFeeTrend,omitempty"`
}

func (f *eip1559Fees) tier(i int) *eip1559Fee {
	switch i {
	case 0:
		return f.Low
	case 1:
		return f.Medium
	case 2:
		return f.High
	default:
		return f.Instant
	}
}

// oneInchFee mirrors bchain/coins/eth/oneinchfees.go. Unlike Infura's Gwei floats
// these are wei decimal strings.
type oneInchFee struct {
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	MaxFeePerGas         string `json:"maxFeePerGas"`
}

type oneInchFees struct {
	BaseFee string     `json:"baseFee"`
	Low     oneInchFee `json:"low"`
	Medium  oneInchFee `json:"medium"`
	High    oneInchFee `json:"high"`
	Instant oneInchFee `json:"instant"`
}

// remapped applies the tier shift oneInchFeeProvider.processData performs, so the
// comparison shows what Blockbook would actually serve if the provider were
// switched: 1inch medium/high/instant become Suite's low/medium/high, and 1inch's
// own low tier is discarded as too conservative.
func (f *oneInchFees) remapped() ([]tierFee, float64) {
	out := make([]tierFee, len(tierNames))
	for i, t := range []oneInchFee{f.Medium, f.High, f.Instant} {
		maxFee, _ := parseWei(t.MaxFeePerGas)
		tip, _ := parseWei(t.MaxPriorityFeePerGas)
		out[i] = tierFee{maxFee: maxFee, priorityFee: tip}
	}
	// The provider's own view of the base fee: not an estimate at all, so any gap
	// from the block's real base fee is staleness rather than error.
	base, _ := parseWei(f.BaseFee)
	return out, base
}

type estimateFeeRes struct {
	FeePerTx   string       `json:"feePerTx,omitempty"`
	FeePerUnit string       `json:"feePerUnit,omitempty"`
	FeeLimit   string       `json:"feeLimit,omitempty"`
	Eip1559    *eip1559Fees `json:"eip1559,omitempty"`
}

type ethereumGasData struct {
	BaseFeePerGas string `json:"baseFeePerGas,omitempty"`
	BlockGasUsed  string `json:"blockGasUsed,omitempty"`
	BlockGasLimit string `json:"blockGasLimit,omitempty"`
}

type newBlock struct {
	Height  uint32           `json:"height"`
	Hash    string           `json:"hash"`
	EVMData *ethereumGasData `json:"evmData"`
}

type wsResponse struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

// ---------------------------------------------------------------- REST types

type restBlock struct {
	Height     int   `json:"height"`
	Time       int64 `json:"time"`
	TxCount    int   `json:"txCount"`
	Page       int   `json:"page"`
	TotalPages int   `json:"totalPages"`
	Txs        []struct {
		Txid             string `json:"txid"`
		EthereumSpecific *struct {
			GasUsed              json.Number `json:"gasUsed"`
			GasLimit             json.Number `json:"gasLimit"`
			GasPrice             string      `json:"gasPrice"`
			EffectiveGasPrice    string      `json:"effectiveGasPrice"`
			BaseFeePerGas        string      `json:"baseFeePerGas"`
			MaxFeePerGas         string      `json:"maxFeePerGas"`
			MaxPriorityFeePerGas string      `json:"maxPriorityFeePerGas"`
		} `json:"ethereumSpecific"`
	} `json:"txs"`
}

// ---------------------------------------------------------------- block stats

// txFee is one mined transaction reduced to the two numbers that price a block:
// what it effectively paid per gas, and how much of the block it occupied.
type txFee struct {
	effective float64 // wei per gas
	tip       float64 // effective - baseFee, i.e. what the proposer actually earned
	gasUsed   float64
}

// blockStats is everything the analysis needs from one block.
type blockStats struct {
	height   int
	time     int64
	baseFee  float64
	gasUsed  float64
	txs      []txFee
	clearMin float64 // lowest effective gas price included
	clearP10 float64 // 10th percentile of included effective gas prices
	medEff   float64
	// reward holds the feeHistory reward percentiles reconstructed for this block,
	// in feeHistoryPercentiles order.
	reward []float64
}

func parseWei(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	b, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return 0, false
	}
	f, _ := new(big.Float).SetInt(b).Float64()
	return f, true
}

func parseWeiInt(s string) *big.Int {
	if s == "" {
		return nil
	}
	b, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil
	}
	return b
}

// summarize reduces a fetched block to blockStats, reproducing geth's
// eth_feeHistory reward computation so the on-chain estimate can be recomputed
// offline. geth sorts a block's txs by effective tip ascending, walks the
// cumulative gas used, and reports the tip of the tx at which the cumulative
// first crosses percentile/100 of the block's total gas.
func summarize(b *restBlock) *blockStats {
	s := &blockStats{height: b.Height, time: b.Time, reward: make([]float64, len(feeHistoryPercentiles))}
	for i := range b.Txs {
		e := b.Txs[i].EthereumSpecific
		if e == nil {
			continue
		}
		eff, ok := parseWei(e.EffectiveGasPrice)
		if !ok {
			continue
		}
		base, _ := parseWei(e.BaseFeePerGas)
		gas, err := e.GasUsed.Float64()
		if err != nil {
			continue
		}
		// Every tx in a block shares the block's base fee; take it from the first
		// tx that reports one rather than trusting any single row.
		if s.baseFee == 0 && base > 0 {
			s.baseFee = base
		}
		tip := eff - base
		if tip < 0 {
			tip = 0
		}
		s.txs = append(s.txs, txFee{effective: eff, tip: tip, gasUsed: gas})
		s.gasUsed += gas
	}
	if len(s.txs) == 0 {
		return s
	}

	eff := make([]float64, len(s.txs))
	for i, t := range s.txs {
		eff[i] = t.effective
	}
	sort.Float64s(eff)
	s.clearMin = eff[0]
	s.clearP10 = percentile(eff, 10)
	s.medEff = percentile(eff, 50)

	byTip := make([]txFee, len(s.txs))
	copy(byTip, s.txs)
	sort.Slice(byTip, func(i, j int) bool { return byTip[i].tip < byTip[j].tip })
	for pi, p := range feeHistoryPercentiles {
		threshold := s.gasUsed * p / 100
		var cum float64
		s.reward[pi] = byTip[len(byTip)-1].tip
		for _, t := range byTip {
			cum += t.gasUsed
			if cum >= threshold {
				s.reward[pi] = t.tip
				break
			}
		}
	}
	return s
}

// percentile returns the p-th percentile of an already-sorted slice by nearest rank.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

// onchainEstimate recomputes what Blockbook's on-chain path would have returned
// for a quote anchored at height h: the mean of each reward percentile over the
// trailing feeHistoryBlocks window, with maxFee = 2*baseFee + tip.
// Returns nil when the window is not fully collected.
func onchainEstimate(blocks map[int]*blockStats, h int) []tierFee {
	base, ok := blocks[h]
	if !ok || base.baseFee == 0 {
		return nil
	}
	out := make([]tierFee, len(feeHistoryPercentiles))
	for i := range feeHistoryPercentiles {
		var sum float64
		var rows int
		for j := 0; j < feeHistoryBlocks; j++ {
			b, ok := blocks[h-j]
			if !ok || len(b.txs) == 0 {
				continue
			}
			sum += b.reward[i]
			rows++
		}
		if rows == 0 {
			return nil
		}
		// Blockbook averages with integer division on int64 wei; at these magnitudes
		// the float mean differs only in the sub-wei digit.
		tip := sum / float64(rows)
		out[i] = tierFee{
			maxFee:      eip1559BaseFeeMultiplier*base.baseFee + tip,
			priorityFee: tip,
		}
	}
	return out
}

type tierFee struct {
	maxFee      float64
	priorityFee float64
}

// ---------------------------------------------------------------- collection

// quote is one estimateFee response anchored to the chain height at which it was asked.
type quote struct {
	at      time.Time
	height  int // tip height when the quote was taken
	baseFee float64
	fees    *eip1559Fees
	source  string // fingerprinted: "onchain", "provider" or "unknown"
	// oneInch is the competing provider's answer as of the same block, so the two
	// are compared on identical market conditions rather than across time.
	oneInch     []tierFee
	oneInchBase float64
}

type collector struct {
	chain   string
	http    *http.Client
	verbose bool

	mu     sync.Mutex
	blocks map[int]*blockStats
	quotes []quote
	errs   []string
	// want holds heights some quote needs; a single drainer fetches them as the
	// tip reaches them. Fast chains (rhc mines ~10 blocks/s) would otherwise pull
	// every block and trip Cloudflare's rate limit.
	want map[int]bool
	tip  int

	oneInch     []tierFee
	oneInchBase float64
	oneInchErr  string
}

func newCollector(chain string, verbose bool) *collector {
	return &collector{
		chain:   chain,
		verbose: verbose,
		blocks:  map[int]*blockStats{},
		want:    map[int]bool{},
		// Blockbook is behind Cloudflare, which drops REST callers that loop hard;
		// one fetch per block per chain stays well inside that.
		http: &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *collector) logf(format string, a ...interface{}) {
	if c.verbose {
		fmt.Printf("  [%s] %s\n", c.chain, fmt.Sprintf(format, a...))
	}
}

func (c *collector) note(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	c.mu.Lock()
	c.errs = append(c.errs, msg)
	c.mu.Unlock()
	fmt.Fprintf(os.Stderr, "  [%s] %s\n", c.chain, msg)
}

func (c *collector) getJSON(u string, out interface{}) error {
	resp, err := c.http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		// Cloudflare answers rate limiting with an HTML challenge, not JSON; surface
		// that as a throttling signal rather than a Blockbook fault. The body is
		// flattened so a multi-line error page cannot hide the chain prefix.
		return fmt.Errorf("HTTP %d from %s (%.80s)", resp.StatusCode, u, flatten(string(body)))
	}
	return json.Unmarshal(body, out)
}

func (c *collector) tipHeight() (int, error) {
	var st struct {
		Blockbook struct {
			BestHeight int  `json:"bestHeight"`
			InSync     bool `json:"inSync"`
		} `json:"blockbook"`
	}
	if err := c.getJSON("https://"+host(c.chain)+"/api/v2/", &st); err != nil {
		return 0, err
	}
	return st.Blockbook.BestHeight, nil
}

// fetchBlock pulls every page of a block and stores its summary.
func (c *collector) fetchBlock(h int) error {
	c.mu.Lock()
	_, done := c.blocks[h]
	c.mu.Unlock()
	if done {
		return nil
	}
	base := fmt.Sprintf("https://%s/api/v2/block/%d", host(c.chain), h)
	var agg restBlock
	page := 1
	for {
		u := base
		if page > 1 {
			u = fmt.Sprintf("%s?page=%d", base, page)
		}
		var b restBlock
		if err := c.getJSON(u, &b); err != nil {
			return err
		}
		if page == 1 {
			agg = b
		} else {
			agg.Txs = append(agg.Txs, b.Txs...)
		}
		if b.TotalPages <= page {
			break
		}
		page++
	}
	s := summarize(&agg)
	c.mu.Lock()
	c.blocks[h] = s
	c.mu.Unlock()
	c.logf("block %d: %d txs, base %.4f gwei, clearP10 %.4f gwei", h, len(s.txs), s.baseFee/gwei, s.clearP10/gwei)
	return nil
}

// requestWindow marks the blocks a quote at height h will be scored against:
// the trailing feeHistory window plus the following depth blocks.
func (c *collector) requestWindow(h, depth int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for j := h - feeHistoryBlocks + 1; j <= h+depth; j++ {
		if _, have := c.blocks[j]; !have {
			c.want[j] = true
		}
	}
}

// drainWanted fetches requested blocks that the tip has already passed, one at a
// time with a gap between them so a burst of quotes cannot turn into a burst of
// REST calls.
func (c *collector) drainWanted(ctx context.Context, gap time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(gap):
		}
		c.mu.Lock()
		next, found := 0, false
		for h := range c.want {
			if h <= c.tip && (!found || h < next) {
				next, found = h, true
			}
		}
		if found {
			delete(c.want, next)
		}
		c.mu.Unlock()
		if !found {
			continue
		}
		if err := c.fetchBlock(next); err != nil {
			c.note("block %d: %v", next, err)
			// Give Cloudflare room rather than hammering through a challenge.
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// pollOneInch keeps the latest 1inch gas price for this chain, so a quote can be
// compared against the competing provider under identical market conditions.
// key is resolved per poll rather than captured once, so replacing a rejected key
// on disk starts producing data without restarting a long run.
func (c *collector) pollOneInch(ctx context.Context, key func() string, period time.Duration) {
	id, ok := oneInchChainIDs[c.chain]
	if !ok {
		return
	}
	u := fmt.Sprintf("https://api.1inch.dev/gas-price/v1.5/%d", id)
	for {
		apiKey := key()
		if apiKey == "" {
			select {
			case <-ctx.Done():
				return
			case <-time.After(period):
			}
			continue
		}
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := c.http.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var f oneInchFees
				if err := json.Unmarshal(body, &f); err == nil {
					tiers, base := f.remapped()
					c.mu.Lock()
					recovered := c.oneInchErr != ""
					c.oneInch, c.oneInchBase, c.oneInchErr = tiers, base, ""
					c.mu.Unlock()
					// Log the recovery too: silence on success would otherwise be
					// indistinguishable from silence on a repeated failure.
					if recovered {
						fmt.Printf("  [%s] 1inch recovered\n", c.chain)
					}
				} else {
					c.setOneInchErr("decode: " + err.Error())
				}
			} else {
				// 404 here just means 1inch does not cover this chain; record it
				// once rather than failing the run.
				c.setOneInchErr(fmt.Sprintf("HTTP %d: %.80s", resp.StatusCode, flatten(string(body))))
			}
		} else if ctx.Err() == nil {
			c.setOneInchErr(err.Error())
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(period):
		}
	}
}

func (c *collector) setOneInchErr(msg string) {
	c.mu.Lock()
	changed := c.oneInchErr != msg
	c.oneInchErr = msg
	c.oneInch = nil
	c.mu.Unlock()
	if changed {
		fmt.Fprintf(os.Stderr, "  [%s] 1inch unavailable: %s\n", c.chain, msg)
	}
}

// backfill collects the n blocks ending at the current tip. It yields the
// algorithm-vs-reality comparison immediately; it cannot produce historical
// provider quotes, which only exist at the moment they are asked for.
func (c *collector) backfill(ctx context.Context, n int) {
	tip, err := c.tipHeight()
	if err != nil {
		c.note("tip: %v", err)
		return
	}
	fmt.Printf("[%s] backfilling %d blocks ending at %d\n", c.chain, n, tip)
	for i := 0; i < n; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := c.fetchBlock(tip - i); err != nil {
			c.note("block %d: %v", tip-i, err)
			// Back off rather than hammering through a Cloudflare challenge.
			time.Sleep(3 * time.Second)
		}
	}
}

// ---------------------------------------------------------------- live sampling

// estimateFeeSpecific is a plain native transfer between two code-less addresses.
// The call must not revert: a failed estimate writes an ERROR line into the public
// server's log (server/websocket.go), and this probe runs against production.
var estimateFeeSpecific = map[string]interface{}{
	"from":  "0x000000000000000000000000000000000000dEaD",
	"to":    "0x000000000000000000000000000000000000dEaD",
	"value": "0x0",
}

// dialer honours HTTPS_PROXY; the zero-value Dialer would ignore it and dial direct.
var dialer = websocket.Dialer{
	HandshakeTimeout: 20 * time.Second,
	Proxy:            http.ProxyFromEnvironment,
	NetDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	},
}

// live opens one websocket per chain, takes a quote on every new block, and
// fetches the blocks that follow so each quote can be scored against them.
func (c *collector) live(ctx context.Context, depth int, minQuote time.Duration, gap time.Duration) {
	wsURL := (&url.URL{Scheme: "wss", Host: host(c.chain), Path: "/websocket"}).String()
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		c.note("dial %s: %v", wsURL, err)
		return
	}
	defer conn.Close()
	fmt.Printf("[%s] connected to %s\n", c.chain, wsURL)

	var wmu sync.Mutex
	send := func(id, method string, params interface{}) error {
		wmu.Lock()
		defer wmu.Unlock()
		return conn.WriteJSON(map[string]interface{}{"id": id, "method": method, "params": params})
	}

	if err := send("sub", "subscribeNewBlock", map[string]interface{}{}); err != nil {
		c.note("subscribe: %v", err)
		return
	}

	// Cloudflare closes an idle Blockbook websocket at ~100 s with code 1006, which
	// looks like a silent disconnect rather than an error. An app-level ping keeps
	// the tunnel open between blocks on slow chains.
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := send("ping", "ping", map[string]interface{}{}); err != nil {
					return
				}
			}
		}
	}()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	go c.drainWanted(ctx, gap)

	var pendingHeight int
	var pendingBase float64
	var seq int
	var lastQuote time.Time

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() == nil {
				c.note("read: %v", err)
			}
			return
		}
		var m wsResponse
		if err := json.Unmarshal(payload, &m); err != nil {
			continue
		}
		switch {
		case m.ID == "sub":
			var nb newBlock
			if err := json.Unmarshal(m.Data, &nb); err != nil || nb.Height == 0 {
				continue
			}
			h := int(nb.Height)
			c.mu.Lock()
			if h > c.tip {
				c.tip = h
			}
			c.mu.Unlock()
			pendingBase = 0
			if nb.EVMData != nil {
				pendingBase, _ = parseWei(nb.EVMData.BaseFeePerGas)
			}
			// Quote at most once per minQuote. Sub-second chains would otherwise
			// produce thousands of near-identical quotes and the REST fetches to
			// score them all.
			if time.Since(lastQuote) < minQuote {
				continue
			}
			lastQuote = time.Now()
			pendingHeight = h
			seq++
			// Ask the moment a new tip lands, so the quote and the blocks it is
			// scored against are unambiguously ordered.
			if err := send(fmt.Sprintf("fee-%d-%d", h, seq), "estimateFee",
				map[string]interface{}{"blocks": []int{1}, "specific": estimateFeeSpecific}); err != nil {
				c.note("estimateFee: %v", err)
				return
			}
			c.requestWindow(h, depth)

		case strings.HasPrefix(m.ID, "fee-"):
			var res []estimateFeeRes
			if err := json.Unmarshal(m.Data, &res); err != nil || len(res) == 0 {
				c.note("estimateFee response: %.200s", string(m.Data))
				continue
			}
			f := res[0].Eip1559
			if f == nil {
				c.note("estimateFee returned no eip1559 block (feePerUnit %s) - chain likely has eip1559Fees disabled", res[0].FeePerUnit)
				continue
			}
			base, _ := parseWei(f.BaseFeePerGas)
			c.mu.Lock()
			oi, oiBase := c.oneInch, c.oneInchBase
			c.mu.Unlock()
			q := quote{at: time.Now(), height: pendingHeight, baseFee: base, fees: f,
				source: fingerprint(f), oneInch: oi, oneInchBase: oiBase}
			if pendingBase > 0 && base > 0 && pendingBase != base {
				c.logf("quote base %.4f gwei differs from pushed header base %.4f gwei", base/gwei, pendingBase/gwei)
			}
			c.mu.Lock()
			c.quotes = append(c.quotes, q)
			c.mu.Unlock()
			c.logf("quote @%d source=%s base=%.4f gwei high.maxFee=%.4f gwei", q.height, q.source, base/gwei, feeGwei(f.High, true))
		}
	}
}

func feeGwei(f *eip1559Fee, maxFee bool) float64 {
	if f == nil {
		return 0
	}
	s := f.MaxPriorityFeePerGas
	if maxFee {
		s = f.MaxFeePerGas
	}
	v, _ := parseWei(s)
	return v / gwei
}

// fingerprint tells the two fee sources apart from the response alone, without
// backend access. The on-chain path always emits an instant tier and, by
// construction, maxFee = 2*baseFee + tip exactly; Infura emits no instant tier and
// its own pre-padded maxFee. This also catches a provider silently falling back
// on-chain when its cache goes stale.
func fingerprint(f *eip1559Fees) string {
	base := parseWeiInt(f.BaseFeePerGas)
	if base == nil {
		return "unknown"
	}
	if f.Instant == nil {
		return "provider"
	}
	twiceBase := new(big.Int).Mul(base, big.NewInt(eip1559BaseFeeMultiplier))
	for i := 0; i < len(tierNames); i++ {
		t := f.tier(i)
		if t == nil {
			return "provider"
		}
		maxFee, tip := parseWeiInt(t.MaxFeePerGas), parseWeiInt(t.MaxPriorityFeePerGas)
		if maxFee == nil || tip == nil {
			return "unknown"
		}
		if new(big.Int).Sub(maxFee, twiceBase).Cmp(tip) != 0 {
			return "provider"
		}
	}
	return "onchain"
}

// ---------------------------------------------------------------- analysis

// sample is one (fee source, tier) pair at one height, scored against the blocks
// that followed. Emitting the served quote, 1inch and Blockbook's own on-chain
// algorithm as sibling rows lets them be compared on identical market conditions.
type sample struct {
	chain  string
	height int
	tier   string
	// provider is which answer this row is: "served" (what the host returned),
	// "1inch", or "onchain" (what Blockbook's own algorithm would have said).
	provider string
	source   string // fingerprint of the served quote; empty on the other rows
	baseFee  float64
	// reportedBase is the base fee this particular source claimed. The base fee is
	// consensus-determined, so a gap from the block's own value is staleness, never
	// a bad estimate - which makes it worth recording per source rather than once.
	reportedBase float64
	maxFee       float64
	tip          float64
	// Suite applies per-coin clamps to whatever Blockbook returns, so these are
	// what the user actually sees.
	suiteMaxFee float64
	suiteTip    float64
	clampBound  bool
	// inclusion: first block at or after height+1 that the quote would have cleared.
	includedAt int // 0 = never within the horizon
	// overpay at the block of inclusion.
	overpayP10 float64
	overpayMin float64
	// mktTip is the priority fee that would actually have sufficed in the block
	// that included the quote: its clearing price less its base fee. Comparing the
	// served tip against this is the whole point of the audit.
	mktTip float64
}

// scorable reports whether a quote's outcome is known. A quote taken near the end
// of the sampling window has no blocks after it yet; counting it as "not included"
// would deflate the inclusion rate, so it is dropped instead. Once a quote has been
// included the rest of the window no longer matters.
func scorable(blocks map[int]*blockStats, h, depth int, includedAt int) bool {
	if includedAt > 0 {
		return true
	}
	for i := 1; i <= depth; i++ {
		if _, ok := blocks[h+i]; !ok {
			return false
		}
	}
	return true
}

// score simulates the quote against blocks h+1..h+depth. A tx is eligible for a
// block only if its maxFee covers that block's base fee; what it then pays is
// min(maxFee, baseFee+tip), and it clears if that reaches the block's clearing price.
func score(s *sample, blocks map[int]*blockStats, depth int) {
	for i := 1; i <= depth; i++ {
		b, ok := blocks[s.height+i]
		if !ok || len(b.txs) == 0 {
			continue
		}
		if s.suiteMaxFee < b.baseFee {
			continue
		}
		effective := math.Min(s.suiteMaxFee, b.baseFee+s.suiteTip)
		if effective < b.clearP10 {
			continue
		}
		s.includedAt = b.height
		s.mktTip = b.clearP10 - b.baseFee
		if s.mktTip < 0 {
			s.mktTip = 0
		}
		if b.clearP10 > 0 {
			s.overpayP10 = effective / b.clearP10
		}
		if b.clearMin > 0 {
			s.overpayMin = effective / b.clearMin
		}
		return
	}
}

// applySuiteClamps mirrors trezor-suite EthereumFeeLevels.load: maxFeePerGas is
// raised to the greater of minFee and minPriorityFee, and the tip is floored at
// minPriorityFee but never allowed above maxFeePerGas. Suite does not recompute
// maxFeePerGas from the base fee; it passes Blockbook's value through.
func applySuiteClamps(s *sample, c suiteClamp) {
	minFee := c.minGwei * gwei
	minPriority := c.minPriorityGwei * gwei
	s.suiteMaxFee = math.Max(minFee, math.Max(s.maxFee, minPriority))
	s.suiteTip = math.Max(minPriority, math.Min(s.suiteMaxFee, s.tip))
	s.clampBound = s.suiteMaxFee != s.maxFee || s.suiteTip != s.tip
}

func (c *collector) samples(depth int, synthetic bool) []sample {
	c.mu.Lock()
	defer c.mu.Unlock()
	clamp := clampFor(c.chain)
	var out []sample
	var dropped int

	quotes := c.quotes
	if synthetic {
		// Backfill mode has no served quotes; score Blockbook's own algorithm at
		// every collected height instead, which answers whether the on-chain path
		// itself prices blocks correctly.
		quotes = nil
		heights := make([]int, 0, len(c.blocks))
		for h := range c.blocks {
			heights = append(heights, h)
		}
		sort.Ints(heights)
		for _, h := range heights {
			cf := onchainEstimate(c.blocks, h)
			if cf == nil {
				continue
			}
			f := &eip1559Fees{BaseFeePerGas: fmt.Sprintf("%.0f", c.blocks[h].baseFee)}
			tiers := []*eip1559Fee{}
			for _, t := range cf {
				tiers = append(tiers, &eip1559Fee{
					MaxFeePerGas:         fmt.Sprintf("%.0f", t.maxFee),
					MaxPriorityFeePerGas: fmt.Sprintf("%.0f", t.priorityFee),
				})
			}
			f.Low, f.Medium, f.High, f.Instant = tiers[0], tiers[1], tiers[2], tiers[3]
			quotes = append(quotes, quote{height: h, baseFee: c.blocks[h].baseFee, fees: f, source: "reconstructed"})
		}
	}

	// emit scores one candidate answer and keeps it only if its outcome is known.
	emit := func(q quote, i int, provider, source string, f tierFee, reportedBase float64) {
		if f.maxFee == 0 && f.priorityFee == 0 && provider != "served" {
			return
		}
		s := sample{
			chain:        c.chain,
			height:       q.height,
			tier:         tierNames[i],
			provider:     provider,
			source:       source,
			baseFee:      q.baseFee,
			reportedBase: reportedBase,
			maxFee:       f.maxFee,
			tip:          f.priorityFee,
		}
		applySuiteClamps(&s, clamp)
		score(&s, c.blocks, depth)
		if !scorable(c.blocks, s.height, depth, s.includedAt) {
			dropped++
			return
		}
		out = append(out, s)
	}

	for _, q := range quotes {
		cf := onchainEstimate(c.blocks, q.height)
		onchainBase := q.baseFee
		if b, ok := c.blocks[q.height]; ok && b.baseFee > 0 {
			onchainBase = b.baseFee
		}
		for i := range tierNames {
			if t := q.fees.tier(i); t != nil && !synthetic {
				maxFee, _ := parseWei(t.MaxFeePerGas)
				tip, _ := parseWei(t.MaxPriorityFeePerGas)
				emit(q, i, "served", q.source, tierFee{maxFee: maxFee, priorityFee: tip}, q.baseFee)
			}
			if cf != nil {
				// The on-chain path reads the base fee off the chain, so its reported
				// value is the block's own by construction.
				emit(q, i, "onchain", "", cf[i], onchainBase)
			}
			if i < len(q.oneInch) {
				emit(q, i, "1inch", "", q.oneInch[i], q.oneInchBase)
			}
		}
	}
	if dropped > 0 {
		fmt.Printf("[%s] dropped %d tier-samples whose %d-block outcome window was incomplete\n", c.chain, dropped, depth)
	}
	return out
}

// ---------------------------------------------------------------- reporting

func med(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return percentile(s, 50)
}

func p90(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return percentile(s, 90)
}

func writeTSV(dir string, all []sample) (string, error) {
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "feeaudit.tsv")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, strings.Join([]string{
		"chain", "height", "tier", "provider", "source", "base_gwei", "reported_base_gwei",
		"maxfee_gwei", "tip_gwei",
		"suite_maxfee_gwei", "suite_tip_gwei", "clamp_bound",
		"included_at", "blocks_waited", "mkt_tip_gwei", "overpay_p10", "overpay_min",
	}, "\t"))
	for _, s := range all {
		waited := 0
		if s.includedAt > 0 {
			waited = s.includedAt - s.height
		}
		// Only the served row has a fingerprinted source; a dash keeps the column
		// visible in tools that collapse empty fields.
		source := s.source
		if source == "" {
			source = "-"
		}
		fmt.Fprintf(f, "%s\t%d\t%s\t%s\t%s\t%.9f\t%.9f\t%.9f\t%.9f\t%.9f\t%.9f\t%t\t%d\t%d\t%.9f\t%.4f\t%.4f\n",
			s.chain, s.height, s.tier, s.provider, source, s.baseFee/gwei, s.reportedBase/gwei,
			s.maxFee/gwei, s.tip/gwei,
			s.suiteMaxFee/gwei, s.suiteTip/gwei, s.clampBound,
			s.includedAt, waited, s.mktTip/gwei, s.overpayP10, s.overpayMin)
	}
	return path, nil
}

// writeBlocksTSV dumps per-block ground truth: the chain's own base fee and what
// transactions in that block actually paid. It is what every quote is scored
// against, and the only series in the output that owes nothing to any provider.
func writeBlocksTSV(dir string, collectors []*collector) (string, error) {
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "blocks.tsv")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fmt.Fprintln(f, strings.Join([]string{
		"chain", "height", "time", "txs", "gas_used",
		"base_gwei", "clear_min_gwei", "clear_p10_gwei", "median_gwei",
		"mkt_tip_p10_gwei", "reward_p20_gwei", "reward_p70_gwei", "reward_p90_gwei", "reward_p99_gwei",
	}, "\t"))
	rows := 0
	for _, c := range collectors {
		c.mu.Lock()
		heights := make([]int, 0, len(c.blocks))
		for h := range c.blocks {
			heights = append(heights, h)
		}
		sort.Ints(heights)
		for _, h := range heights {
			b := c.blocks[h]
			if len(b.txs) == 0 {
				continue
			}
			tip := b.clearP10 - b.baseFee
			if tip < 0 {
				tip = 0
			}
			r := func(i int) float64 {
				if i < len(b.reward) {
					return b.reward[i] / gwei
				}
				return 0
			}
			fmt.Fprintf(f, "%s\t%d\t%d\t%d\t%.0f\t%.9f\t%.9f\t%.9f\t%.9f\t%.9f\t%.9f\t%.9f\t%.9f\t%.9f\n",
				c.chain, b.height, b.time, len(b.txs), b.gasUsed,
				b.baseFee/gwei, b.clearMin/gwei, b.clearP10/gwei, b.medEff/gwei,
				tip/gwei, r(0), r(1), r(2), r(3))
			rows++
		}
		c.mu.Unlock()
	}
	if rows == 0 {
		return "", nil
	}
	return path, nil
}

func report(all []sample) {
	type key struct{ chain, tier, provider string }
	groups := map[key][]sample{}
	chains := []string{}
	seen := map[string]bool{}
	for _, s := range all {
		groups[key{s.chain, s.tier, s.provider}] = append(groups[key{s.chain, s.tier, s.provider}], s)
		if !seen[s.chain] {
			seen[s.chain] = true
			chains = append(chains, s.chain)
		}
	}
	// "served" first so each tier reads as "what we ship, then the alternatives".
	providers := []string{"served", "1inch", "onchain"}

	fmt.Printf("\n%-6s %-8s %-8s %5s %11s %11s %8s %8s %7s %7s %6s\n",
		"chain", "tier", "provider", "n", "tip_gwei", "mktTip_gwei", "paid", "shown", "incl1", "inclAll", "clamp")
	fmt.Println(strings.Repeat("-", 106))
	for _, chain := range chains {
		for _, tier := range tierNames {
			printed := false
			for _, provider := range providers {
				v := groups[key{chain, tier, provider}]
				if len(v) == 0 {
					continue
				}
				var tips, mkts, overpays, shown []float64
				var incl1, inclAny, clamped int
				for _, s := range v {
					// The tip, not maxFeePerGas, is what a sender actually pays above
					// the base fee: EIP-1559 refunds the rest.
					tips = append(tips, s.suiteTip/gwei)
					if s.clampBound {
						clamped++
					}
					if s.includedAt == 0 {
						continue
					}
					inclAny++
					mkts = append(mkts, s.mktTip/gwei)
					if s.includedAt == s.height+1 {
						incl1++
					}
					if s.overpayP10 > 0 {
						overpays = append(overpays, s.overpayP10)
					}
					// Suite displays and reserves maxFeePerGas * gasLimit, so the
					// ceiling is what the user sees even though it is refunded.
					if going := s.baseFee + s.mktTip; going > 0 {
						shown = append(shown, s.suiteMaxFee/going)
					}
				}
				n := len(v)
				fmt.Printf("%-6s %-8s %-8s %5d %11.6f %11.6f %7.1fx %7.1fx %6.0f%% %6.0f%% %5.0f%%\n",
					chain, orBlank(tier, printed), provider, n, med(tips), med(mkts),
					med(overpays), med(shown),
					100*float64(incl1)/float64(n), 100*float64(inclAny)/float64(n),
					100*float64(clamped)/float64(n))
				printed = true
			}
		}
	}
	fmt.Println("\nprovider: served = what the host returned now | 1inch = what it would return on the 1inch provider")
	fmt.Println("          onchain = what Blockbook's own eth_feeHistory algorithm would have said")
	fmt.Println("tip_gwei = quoted priority fee after Suite's clamps | mktTip = the tip that actually sufficed")
	fmt.Println("paid     = effective price really paid / the going rate (EIP-1559 refunds the rest)")
	fmt.Println("shown    = what Suite displays and reserves (maxFeePerGas x gasLimit) / the going rate")
	fmt.Println("incl1    = share that would have cleared the very next block; inclAll = within the -depth horizon")
}

// orBlank blanks a repeated row label so each tier reads as one block.
func orBlank(s string, printed bool) string {
	if printed {
		return ""
	}
	return s
}

// ---------------------------------------------------------------- validation

// validate proves the offline eth_feeHistory reconstruction by asking a real node
// for the same window and comparing rewards wei-for-wei. Without this the
// counterfactual column is just an assertion; with it, the provider's numbers can
// be compared against Blockbook's own algorithm on chains that never serve it.
func (c *collector) validate(rpcURL string) error {
	body := `{"jsonrpc":"2.0","id":1,"method":"eth_feeHistory","params":["0x` +
		fmt.Sprintf("%x", feeHistoryBlocks) + `","latest",[20,70,90,99]]}`
	resp, err := c.http.Post(rpcURL, "application/json", strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Result *struct {
			OldestBlock string     `json:"oldestBlock"`
			Reward      [][]string `json:"reward"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Error != nil {
		return fmt.Errorf("node refused eth_feeHistory: %s", out.Error.Message)
	}
	if out.Result == nil || len(out.Result.Reward) == 0 {
		return fmt.Errorf("node returned no reward rows")
	}
	oldest := new(big.Int)
	if _, ok := oldest.SetString(strings.TrimPrefix(out.Result.OldestBlock, "0x"), 16); !ok {
		return fmt.Errorf("bad oldestBlock %q", out.Result.OldestBlock)
	}

	ok, compared := true, 0
	for j, row := range out.Result.Reward {
		h := int(oldest.Int64()) + j
		if err := c.fetchBlock(h); err != nil {
			// The public node is usually a block or two ahead of Blockbook's index;
			// a height it has not reached yet is a race, not a disagreement.
			fmt.Printf("  [%s] h=%d skipped, not indexed yet\n", c.chain, h)
			continue
		}
		compared++
		c.mu.Lock()
		b := c.blocks[h]
		c.mu.Unlock()
		for i := range feeHistoryPercentiles {
			if i >= len(row) {
				continue
			}
			want := new(big.Int)
			want.SetString(strings.TrimPrefix(row[i], "0x"), 16)
			got := new(big.Int).SetUint64(uint64(b.reward[i]))
			if want.Cmp(got) != 0 {
				ok = false
				fmt.Printf("  [%s] h=%d p%v MISMATCH node=%s reconstructed=%s\n",
					c.chain, h, feeHistoryPercentiles[i], want, got)
			}
		}
		fmt.Printf("  [%s] h=%d txs=%d checked %d percentiles\n", c.chain, h, len(b.txs), len(row))
	}
	if !ok {
		return fmt.Errorf("reconstruction does not match the node")
	}
	if compared == 0 {
		return fmt.Errorf("no block could be compared; Blockbook is behind %s", rpcURL)
	}
	fmt.Printf("[%s] reconstruction matches %s over %d of %d blocks\n",
		c.chain, rpcURL, compared, len(out.Result.Reward))
	return nil
}

// ---------------------------------------------------------------- main

func main() {
	chains := flag.String("chains", strings.Join(defaultChains, ","), "comma-separated Blockbook chains (host is <chain>.trezor.io)")
	duration := flag.Duration("duration", 30*time.Minute, "live sampling window; ignored when -backfill is set")
	backfill := flag.Int("backfill", 0, "instead of sampling live, analyse the last N blocks (no served quotes, on-chain algorithm only)")
	depth := flag.Int("depth", 8, "how many following blocks a quote is given to be included")
	minQuote := flag.Duration("min-quote", 5*time.Second, "minimum gap between quotes on one chain; keeps sub-second chains from flooding the endpoint")
	gap := flag.Duration("fetch-gap", 300*time.Millisecond, "pause between REST block fetches on one chain")
	out := flag.String("out", "", "directory for the per-sample TSV; empty writes no file")
	verbose := flag.Bool("v", false, "log every block and quote as it arrives")
	validateRPC := flag.String("validate", "", "EVM JSON-RPC URL: check the offline eth_feeHistory reconstruction against a real node, then exit (single -chains entry)")
	// The key is read from a file or the environment, never a flag value, so it
	// stays out of shell history and the process list.
	keyFile := flag.String("oneinch-key-file", "", "file holding a 1inch API key; enables the 1inch column (falls back to $ONE_INCH_API_KEY)")
	oneInchPeriod := flag.Duration("oneinch-period", 10*time.Second, "how often to poll api.1inch.dev, matching the provider's configured periodSeconds")
	flag.Parse()

	oneInchKey := func() string { return strings.TrimSpace(os.Getenv("ONE_INCH_API_KEY")) }
	if *keyFile != "" {
		if _, err := os.ReadFile(*keyFile); err != nil {
			fmt.Fprintf(os.Stderr, "reading -oneinch-key-file: %v\n", err)
			os.Exit(2)
		}
		oneInchKey = func() string {
			b, err := os.ReadFile(*keyFile)
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(b))
		}
	}
	oneInchEnabled := oneInchKey() != ""
	if !oneInchEnabled {
		fmt.Println("no 1inch key given (-oneinch-key-file or $ONE_INCH_API_KEY); comparing served vs on-chain only")
	}

	names := splitAndTrim(*chains)
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "-chains must name at least one chain")
		os.Exit(2)
	}

	if *validateRPC != "" {
		if len(names) != 1 {
			fmt.Fprintln(os.Stderr, "-validate needs exactly one -chains entry, matching the node's chain")
			os.Exit(2)
		}
		if err := newCollector(names[0], *verbose).validate(*validateRPC); err != nil {
			fmt.Fprintf(os.Stderr, "validation failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\ninterrupted, reporting on what was collected so far")
		cancel()
	}()
	if *backfill == 0 {
		go func() {
			t := time.NewTimer(*duration)
			defer t.Stop()
			select {
			case <-t.C:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	collectors := make([]*collector, 0, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		c := newCollector(name, *verbose)
		collectors = append(collectors, c)
		wg.Add(1)
		go func(c *collector, i int) {
			defer wg.Done()
			// Stagger the starts so the chains do not all hit Cloudflare at once.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(i) * 500 * time.Millisecond):
			}
			if *backfill > 0 {
				c.backfill(ctx, *backfill)
				return
			}
			if oneInchEnabled {
				go c.pollOneInch(ctx, oneInchKey, *oneInchPeriod)
			}
			c.live(ctx, *depth, *minQuote, *gap)
		}(c, i)
	}
	wg.Wait()

	var all []sample
	for _, c := range collectors {
		all = append(all, c.samples(*depth, *backfill > 0)...)
	}
	if len(all) == 0 {
		fmt.Fprintln(os.Stderr, "no samples collected")
		os.Exit(1)
	}
	report(all)
	if path, err := writeTSV(*out, all); err != nil {
		fmt.Fprintf(os.Stderr, "writing TSV: %v\n", err)
	} else if path != "" {
		fmt.Printf("\nper-sample rows: %s (%d)\n", path, len(all))
	}
	if path, err := writeBlocksTSV(*out, collectors); err != nil {
		fmt.Fprintf(os.Stderr, "writing blocks TSV: %v\n", err)
	} else if path != "" {
		fmt.Printf("per-block ground truth: %s\n", path)
	}
}

// flatten collapses runs of whitespace so an HTML error page stays on one log line.
func flatten(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func splitAndTrim(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
