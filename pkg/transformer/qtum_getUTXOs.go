package transformer

import (
	"context"
	"fmt"
	"math/big"

	"github.com/labstack/echo"
	"github.com/qtumproject/janus/pkg/eth"
	"github.com/qtumproject/janus/pkg/qtum"
	"github.com/qtumproject/janus/pkg/utils"
	"github.com/shopspring/decimal"
)

type ProxyQTUMGetUTXOs struct {
	*qtum.Qtum
}

func Contains(sl []string, name string) bool {
   for _, v := range sl {
      if v == name {
         return true
      }
   }
   return false
}

var _ ETHProxy = (*ProxyQTUMGetUTXOs)(nil)

func (p *ProxyQTUMGetUTXOs) Method() string {
	return "qtum_getUTXOs"
}

func (p *ProxyQTUMGetUTXOs) Request(req *eth.JSONRPCRequest, c echo.Context) (interface{}, eth.JSONRPCError) {
	var params eth.GetUTXOsRequest
	if err := unmarshalRequest(req.Params, &params); err != nil {
		// TODO: Correct error code?
		return nil, eth.NewInvalidParamsError("couldn't unmarshal request parameters")
	}

	err := params.CheckHasValidValues()
	if err != nil {
		// TODO: Correct error code?
		return nil, eth.NewInvalidParamsError("couldn't validate parameters value")
	}

	return p.request(c.Request().Context(), params)
}

func (p *ProxyQTUMGetUTXOs) request(ctx context.Context, params eth.GetUTXOsRequest) (*eth.GetUTXOsResponse, eth.JSONRPCError) {
	address, err := convertETHAddress(utils.RemoveHexPrefix(params.Address), p.Chain())
	if err != nil {
		return nil, eth.NewInvalidParamsError("couldn't convert Ethereum address to Qtum address")
	}

	req := qtum.GetAddressUTXOsRequest{
		Addresses: []string{address},
	}

	resp, err := p.Qtum.GetAddressUTXOs(ctx, &req)
	if err != nil {
		return nil, eth.NewCallbackError(err.Error())
	}

	blockCount, err := p.Qtum.GetBlockCount(ctx)
	if err != nil {
		return nil, eth.NewCallbackError(err.Error())
	}

	rawMempool, err := p.Qtum.GetRawMempool(ctx)
	if err != nil {
		return nil, eth.NewCallbackError(err.Error())
	}

	var mempoolUxtoHashes []string

	for _, mempoolHash := range *rawMempool {
		qtumTx, err := p.GetRawTransaction(ctx, mempoolHash, false)
		if err != nil {
            return nil, eth.NewCallbackError(err.Error())
        }
        for _, mempoolVin := range qtumTx.Vins {
            mempoolUxtoHashes = append(mempoolUxtoHashes, mempoolVin.ID)
        }

	}
	p.GetDebugLogger().Log("msg", "mempoolUxtoHashes", "mempoolUxtoHashes", mempoolUxtoHashes)

	matureBlockHeight := big.NewInt(int64(p.Qtum.GetMatureBlockHeight()))

	//Convert minSumAmount to Satoshis
	minimumSum := convertFromQtumToSatoshis(params.MinSumAmount)
	queryingAll := minimumSum.Equal(decimal.Zero)

	allUtxoTypes := false
	if len(params.Types) > 0 {
		if params.Types[0] == eth.ALL_UTXO_TYPES {
			allUtxoTypes = true
		}
	} else {
		allUtxoTypes = true
	}

	utxoTypes := map[eth.UTXOScriptType]bool{}
	for _, typ := range params.Types {
		utxoTypes[typ] = true
	}

	var utxos []eth.QtumUTXO
	var minUTXOsSum decimal.Decimal
	for _, utxo := range *resp {
		ethUTXO := toEthResponseType(utxo)
		ethUTXO.Height = uint64(utxo.Height.Int64())
		ethUTXO.ScriptPubKey = utxo.Script
		utxoType := ethUTXO.GetType()
		ethUTXO.Type = utxoType.String()
		ethUTXO.Safe = true
		if !allUtxoTypes {
			if _, ok := utxoTypes[utxoType]; !ok {
				continue
			}
		}

		if Contains(mempoolUxtoHashes, utxo.TXID) {
		    ethUTXO.Safe = false
		}

		// TODO: This doesn't work on regtest coinbase
		if utxo.IsStake || utxo.IsBase {
			matureAt := big.NewInt(utxo.Height.Int64()).Add(
				big.NewInt(utxo.Height.Int64()),
				matureBlockHeight,
			)
			if blockCount.Int.Cmp(matureAt) <= 0 {
				// immature
				ethUTXO.Safe = false
				if !allUtxoTypes {
					if _, ok := utxoTypes[eth.IMMATURE]; !ok {
						continue
					}
				}
			}
		}

		ethUTXO.Confirmations = blockCount.Int64() - utxo.Height.Int64()
		if ethUTXO.Confirmations < 0 {
			panic(fmt.Sprintf("Computed negative confirmations: %d - %d = %d\n", blockCount.Int64(), utxo.Height.Int64(), ethUTXO.Confirmations))
		}
		ethUTXO.Spendable = true

		if ethUTXO.Safe {
			minUTXOsSum = minUTXOsSum.Add(utxo.Satoshis)
		}
		utxos = append(utxos, ethUTXO)
		if !queryingAll && minUTXOsSum.GreaterThanOrEqual(minimumSum) {
			return (*eth.GetUTXOsResponse)(&utxos), nil
		}
	}

	if queryingAll {
		return (*eth.GetUTXOsResponse)(&utxos), nil
	}

	return nil, eth.NewCallbackError("required minimum amount is greater than total amount of UTXOs")
}

func toEthResponseType(utxo qtum.UTXO) eth.QtumUTXO {
	return eth.QtumUTXO{
		Address: utxo.Address,
		TXID:    utxo.TXID,
		Vout:    utxo.OutputIndex,
		Amount:  convertFromSatoshisToQtum(utxo.Satoshis).String(),
	}
}
