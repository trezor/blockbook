package eth

import (
	"encoding/hex"
	"math/big"
	"strconv"
	"strings"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"golang.org/x/crypto/sha3"
)

// nameRegisteredEventSignature is the topic0 of the ENS NameRegistered event.
const nameRegisteredEventSignature = "0xca6abbe9d7f11422cb6ca7629fbf6fe9efb1c621f71ce8f02b9f2a230097404f"

// ENS contract addresses and function selectors (Ethereum mainnet).
const (
	// ENSRegistryAddress is the mainnet ENS registry contract address.
	ENSRegistryAddress = "0x00000000000C2E074eC69A0dFb2997BA6C7d2e1e"
	// ENSBaseRegistrarAddress is the .eth Base Registrar, used to check expiration.
	ENSBaseRegistrarAddress = "0x57f1887a8BF19b14fC0dF6Fd9B2acc9Af147eA85"
	// ENSResolverFunctionSelector is the ENS registry's resolver(bytes32) selector.
	ENSResolverFunctionSelector = "0x0178b8bf"
	// ENSAddrFunctionSelector is the resolver's addr(bytes32) selector.
	ENSAddrFunctionSelector = "0x3b3b57de"
	// ENSExpirationFunctionSelector is the Base Registrar's nameExpires(uint256) selector.
	ENSExpirationFunctionSelector = "0xd6e4fa86"
)

// SetEnsSuffix sets the suffix appended to a decoded ENS name when formatting.
func (p *EthereumParser) SetEnsSuffix(suffix string) {
	p.EnsSuffix = suffix
}

// UseEnsReverseAliases reports whether ENS reverse aliases are recorded and served.
// Opt-in: a chain gets them only with both address_aliases and
// enable_ens_reverse_aliases set. Gating the recording side means toggling this on later
// does not recover labels for blocks already synced without it (no backfill).
func (p *EthereumParser) UseEnsReverseAliases() bool {
	return p.AddressAliases && p.EnableEnsReverseAliases
}

// getEnsRecord parses an ENS record from a transaction log entry.
//
// WARNING: not production-ready, disabled by default (see UseEnsReverseAliases).
// Trusts any log emitter (spoofable labels), does not validate names, and ignores
// expiry/ownership (stale/duplicate labels). Use forward ResolveENS instead.
func getEnsRecord(l *rpcLogWithTxHash) *bchain.AddressAliasRecord {
	if len(l.Topics) == 3 && l.Topics[0] == nameRegisteredEventSignature && len(l.Data) >= 322 {
		address, err := addressFromPaddedHex(l.Topics[2])
		if err != nil {
			return nil
		}
		c, err := strconv.ParseInt(l.Data[194:194+64], 16, 64)
		if err != nil {
			return nil
		}
		const nameStart = 194 + 64
		// int(c)<<1 can overflow: c is attacker-controlled (up to 2^63-1) and Go's
		// signed left shift wraps silently, so de can land anywhere in [0, 257]
		// below nameStart. The lower bound must be the slice
		// start index, not 0 — a de below the start makes l.Data[nameStart:de] a
		// low>high slice expression that panics. Checking de < nameStart guarantees
		// nameStart <= de <= len(l.Data), so the slice below can never panic.
		de := nameStart + (int(c) << 1)
		if de > len(l.Data) || de < nameStart {
			return nil
		}
		b, err := hex.DecodeString(l.Data[nameStart:de])
		if err != nil {
			return nil
		}
		return &bchain.AddressAliasRecord{Address: address, Name: string(b)}
	}
	return nil
}

// ensNameHash computes the ENS namehash of name per the ENS spec.
func ensNameHash(name string) string {
	node := make([]byte, 32)
	if name != "" {
		labels := strings.Split(name, ".")
		for i := len(labels) - 1; i >= 0; i-- {
			labelHash := keccak256([]byte(labels[i]))
			node = keccak256(append(node, labelHash...))
		}
	}
	return "0x" + hex.EncodeToString(node)
}

func keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

func parseENSAddressFromResult(result string) (string, error) {
	if len(result) < 2 || result[:2] != "0x" {
		return "", errors.New("invalid hex result")
	}
	hexData := result[2:]
	if len(hexData) < 64 {
		return "", errors.New("result too short")
	}
	addressHex := hexData[len(hexData)-EthereumAddressHexLength:]
	return "0x" + addressHex, nil
}

func (b *EthereumRPC) ensContracts() (string, string, error) {
	if b.Testnet || b.MainNetChainID != MainNet {
		// ENS contracts are mainnet-only here; avoid calling empty/uninitialized addresses on other networks.
		return "", "", errors.New("ENS contracts not configured for this network")
	}
	return ENSRegistryAddress, ENSBaseRegistrarAddress, nil
}

// ResolveENS resolves ENS domain name to Ethereum address
func (b *EthereumRPC) ResolveENS(name string) (*bchain.ENSResolution, error) {
	glog.Infof("ResolveENS: Starting resolution for %s", name)

	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(name, ".eth") {
		glog.Errorf("ResolveENS: Invalid ENS name %s", name)
		return &bchain.ENSResolution{Name: name, Error: "invalid ENS name"}, errors.New("invalid ENS name")
	}

	// Calculate the namehash for this domain
	node := ensNameHash(name)
	glog.Infof("ResolveENS: Generated node hash %s for %s", node, name)

	registry, _, err := b.ensContracts()
	if err != nil {
		// This avoids empty eth_call targets on L2s while keeping mainnet behavior unchanged
		return &bchain.ENSResolution{Name: name, Error: "ENS not supported on this network"}, err
	}

	// Call resolver(bytes32) on the ENS registry
	callData := map[string]string{
		"to":   registry,
		"data": ENSResolverFunctionSelector + node[2:],
	}
	// Call the resolver function on the ENS registry
	result, err := b.callRpcStringResult("eth_call", callData, "latest")
	if err != nil {
		glog.Errorf("ResolveENS: Registry call failed: %v", err)
		return &bchain.ENSResolution{Name: name, Error: "failed to query ENS registry"}, err
	}
	glog.Infof("ResolveENS: Registry result: %s", result)

	// Parse the resolver address from the result
	//The result is ABI-encoded, we need to extract the address from the last 40 hex characters
	resolverAddr, err := parseENSAddressFromResult(result)
	if err != nil {
		glog.Errorf("ResolveENS: Failed to parse resolver address: %v", err)
		return &bchain.ENSResolution{Name: name, Error: "failed to parse resolver"}, err
	}
	glog.Infof("ResolveENS: Resolver address: %s", resolverAddr)

	if resolverAddr == EthereumZeroAddress {
		glog.Errorf("ResolveENS: No resolver set for %s", name)
		return &bchain.ENSResolution{Name: name, Error: "no resolver set"}, errors.New("no resolver set")
	}

	// Call the addr(bytes32) function on the resolver
	callData = map[string]string{
		"to":   resolverAddr,
		"data": ENSAddrFunctionSelector + node[2:],
	}

	result, err = b.callRpcStringResult("eth_call", callData, "latest")
	if err != nil {
		glog.Errorf("ResolveENS: Resolver call failed: %v", err)
		return &bchain.ENSResolution{Name: name, Error: "failed to query resolver"}, err
	}
	glog.Infof("ResolveENS: Resolver result: %s", result)

	address, err := parseENSAddressFromResult(result)
	if err != nil {
		glog.Errorf("ResolveENS: Failed to parse address: %v", err)
		return &bchain.ENSResolution{Name: name, Error: "failed to parse address"}, err
	}

	if address == EthereumZeroAddress {
		glog.Errorf("ResolveENS: ENS name %s not found", name)
		return &bchain.ENSResolution{Name: name, Error: "ENS name not found"}, errors.New("ENS name not found")
	}

	glog.Infof("ResolveENS: Successfully resolved %s to %s", name, address)
	return &bchain.ENSResolution{Name: name, Address: address}, nil
}

// CheckENSExpiration checks if an ENS domain is expired
func (b *EthereumRPC) CheckENSExpiration(name string) (bool, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	// Only check expiration for .eth domains
	if !strings.HasSuffix(name, ".eth") {
		glog.Infof("CheckENSExpiration: %s is not a .eth domain, skipping expiration check", name)
		return false, nil
	}

	// Extract the label (part before .eth)
	label := strings.TrimSuffix(name, ".eth")
	if strings.Contains(label, ".") {
		// Base Registrar tracks only second-level .eth names; for subdomains, check the parent label.
		parts := strings.Split(label, ".")
		label = parts[len(parts)-1]
	}

	_, registrar, err := b.ensContracts()
	if err != nil {
		return false, err
	}

	// Calculate token ID: keccak256(label)
	labelHash := keccak256([]byte(label))
	tokenID := new(big.Int).SetBytes(labelHash)

	glog.Infof("CheckENSExpiration: Checking expiration for %s (label: %s, tokenID: %s)", name, label, tokenID.String())

	// Pad token ID to 32 bytes (64 hex chars) with leading zeros
	tokenIDHex := hex.EncodeToString(tokenID.Bytes())
	tokenIDPadded := strings.Repeat("0", 64-len(tokenIDHex)) + tokenIDHex

	// Call nameExpires(uint256 id) on the Base Registrar
	callData := map[string]string{
		"to":   registrar,
		"data": ENSExpirationFunctionSelector + tokenIDPadded,
	}

	result, err := b.callRpcStringResult("eth_call", callData, "latest")
	if err != nil {
		// A revert or missing state is expected for unregistered names and on
		// non-archive backends. Skip expiration and let ResolveENS decide the
		// name's fate.
		glog.Warningf("CheckENSExpiration: RPC call failed for %s, skipping expiration: %v", name, err)
		return false, nil
	}

	// Parse the expiration timestamp from the result
	if len(result) < 2 || result[:2] != "0x" {
		glog.Warningf("CheckENSExpiration: invalid hex result for %s, skipping expiration: %s", name, result)
		return false, nil
	}

	// nameExpires returns an ABI-encoded uint256: a 32-byte word, big-endian,
	// left-padded with zeros. Parse it as raw bytes rather than as a hex
	// quantity — hexutil.DecodeBig rejects the leading zeros of a padded word
	// and would fail to decode every real result.
	expiration := new(big.Int).SetBytes(ethcommon.FromHex(result))

	// If nameExpires returns 0 the label is not registered on the Base
	// Registrar (unknown token). Skip expiration and let ResolveENS decide the
	// name's fate rather than treating the zero as "expired".
	if expiration.Sign() == 0 {
		glog.Warningf("CheckENSExpiration: %s has zero expiration, skipping check", name)
		return false, nil
	}

	// Check if expired (current timestamp > expiration timestamp)
	currentTime := big.NewInt(time.Now().Unix())
	isExpired := currentTime.Cmp(expiration) > 0

	expirationTime := time.Unix(expiration.Int64(), 0)
	glog.Infof("CheckENSExpiration: %s expires at %s (expired: %v)", name, expirationTime.String(), isExpired)

	return isExpired, nil
}
