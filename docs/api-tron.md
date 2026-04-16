# Blockbook API V2 - Tron specifics

This document describes Tron-specific behavior in API V2 on top of the generic API documented in [`docs/api.md`](./api.md).

## ID/hash format

For Tron, API V2 returns transaction and block identifiers **without** `0x` prefix:

- `txid`
- `blockHash`
- `previousBlockHash`
- `nextBlockHash`
- status fields like `backend.bestBlockHash` / websocket `bestHash`

Input IDs are accepted in both formats (`<id>` and `0x<id>`), but responses are normalized to no-prefix format.

### Important note about hex-encoded fields

Hex-encoded EVM-like fields inside `coinSpecificData` still use `0x` where applicable (for example `input`, `topics`, `data`, `gasPrice`, `blockNumber`, `status`).

## Tron-specific transaction data (`chainExtraData`)

On Tron, `Tx.chainExtraData` is populated with normalized transaction metadata derived from Tron HTTP APIs (`wallet/gettransactionbyid` + `wallet/gettransactioninfobyid` / `wallet/gettransactioninfobyblocknum`).

The object is omitted when no Tron-specific fields are available.

Schema:

- `contractType` (`string`): raw Tron contract type, e.g. `TriggerSmartContract`, `VoteWitnessContract`, `FreezeBalanceV2Contract`
- `operation` (`string`): normalized operation
  - `vote`
  - `freeze`
  - `unfreeze`
  - `delegate`
  - `undelegate`
  - `transfer`
  - `trc10Transfer`
  - `contractCall`
- `resource` (`string`): `energy` or `bandwidth` (if present on transaction)
- `stakeAmount` (`string`): staked amount (sun), for freeze operations
- `unstakeAmount` (`string`): unstaked amount (sun), for unfreeze operations
- `delegateAmount` (`string`): delegated / undelegated amount (sun)
- `delegateTo` (`string`): destination address for delegate/undelegate operations (base58)
- `assetIssueID` (`string`): TRC10 token ID (when provided by backend)
- `totalFee` (`string`): total transaction fee (sun)
- `energyUsage` (`string`): energy usage from receipt
- `energyUsageTotal` (`string`): total energy usage from receipt
- `energyFee` (`string`): fee paid for energy (sun)
- `bandwidthUsage` (`string`): net/bandwidth usage from receipt
- `bandwidthFee` (`string`): fee paid for bandwidth (sun)
- `result` (`string`): execution result (`SUCCESS`, `FAILED`, etc.)
- `votes` (`array`): only for vote transactions
  - `address` (`string`): voted witness address (base58)
  - `count` (`string`): vote count

## Example (`GET /api/v2/tx/<txid>`)

```json
{
  "txid": "a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302",
  "blockHash": "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff",
  "chainExtraData": {
    "contractType": "TriggerSmartContract",
    "operation": "contractCall",
    "totalFee": "3076500",
    "energyUsageTotal": "14650",
    "bandwidthUsage": "345",
    "result": "SUCCESS"
  }
}
```

## Tron-specific account data (`Address.chainExtraData.payload`)

On Tron, `Address.chainExtraData.payload` also includes staking/governance metadata in `stakingInfo` when available. `stakingInfo` is omitted for non-existent accounts or when required backend staking/account data is temporarily unavailable. If only supplemental reward data is unavailable, `stakingInfo` is still returned and `unclaimedReward` is reported as `0`.

Resource fields:

- `availableStakedBandwidth` (`number`): remaining bandwidth obtained by staking, computed as `max(NetLimit - NetUsed, 0)`
- `totalStakedBandwidth` (`number`): total bandwidth obtained by staking
- `availableFreeBandwidth` (`number`): remaining free bandwidth, computed as `max(freeNetLimit - freeNetUsed, 0)`
- `totalFreeBandwidth` (`number`): total daily free bandwidth
- `availableEnergy` (`number`): remaining energy, computed as `max(EnergyLimit - EnergyUsed, 0)` 
- `totalEnergy` (`number`): total energy obtained by staking

`stakingInfo` schema:

- `stakedBalance` (`string`): total staked TRX in sun (Stake 2.0, bandwidth + energy)
- `stakedBalanceEnergy` (`string`): staked-for-energy amount in sun
- `stakedBalanceBandwidth` (`string`): staked-for-bandwidth amount in sun
- `unstakingBatches` (`array`): pending unstaking batches
  - `amount` (`string`): unstaked amount in sun
  - `expireTime` (`number`): Unix timestamp in **seconds**
- `totalVotingPower` (`string`): total TRON Power owned by the account
- `availableVotingPower` (`string`): remaining TRON Power available for voting, computed as `max(tronPowerLimit - tronPowerUsed, 0)` 
- `votes` (`array`): current vote allocations
  - `address` (`string`): SR address (base58)
  - `voteCount` (`string`): vote count
- `unclaimedReward` (`string`): unclaimed voting reward (sun)
- `delegatedBalanceEnergy` (`string`): delegated staked energy in sun
- `delegatedBalanceBandwidth` (`string`): delegated staked bandwidth in sun
