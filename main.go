package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	confirm "github.com/gagliardetto/solana-go/rpc/sendAndConfirmTransaction"
	"github.com/gagliardetto/solana-go/rpc/ws"

	"github.com/joho/godotenv"
)

type SwapRequest struct {
	Route         Route  `json:"route"`
	WrapUnwrapSOL bool   `json:"wrapUnwrapSOL,omitempty"`
	FeeAccount    string `json:"feeAccount,omitempty"`
	TokenLedger   string `json:"tokenLedger,omitempty"`
	UserPublicKey string `json:"userPublicKey"`
}

type SwapResponse struct {
	SetupTransaction   string `json:"setupTransaction,omitempty"`
	SwapTransaction    string `json:"swapTransaction"`
	CleanupTransaction string `json:"cleanupTransaction,omitempty"`
}

type Quote struct {
	Routes    []Route `json:"data"`
	TimeTaken float64 `json:"timeTaken"`
}

type Route struct {
	InAmount              float64      `json:"inAmount"`
	OutAmount             float64      `json:"outAmount"`
	OutAmountWithSlippage float64      `json:"outAmountWithSlippage"`
	PriceImpactPct        float64      `json:"priceImpactPct"`
	MarketInfos           []MarketInfo `json:"marketInfos"`
}

type MarketInfo struct {
	ID                 string  `json:"id"`
	Label              string  `json:"label"`
	InputMint          string  `json:"inputMint"`
	OutputMint         string  `json:"outputMint"`
	NotEnoughLiquidity bool    `json:"notEnoughLiquidity"`
	InAmount           float64 `json:"inAmount"`
	OutAmount          float64 `json:"outAmount"`
	PriceImpactPct     float64 `json:"priceImpactPct"`
	LpFee              Fee     `json:"lpFee"`
	PlatformFee        Fee     `json:"platformFee"`
}

type Fee struct {
	Amount float64 `json:"amount"`
	Mint   string  `json:"mint"`
	Pct    float64 `json:"pct"`
}

func main() {
	fmt.Println("Hello Jupiter!")

	err := godotenv.Load()
	if err != nil {
		panic(err)
	}
	envWallet := os.Getenv("WALLET_PRIVATE_KEY")

	rpcClient := rpc.New(rpc.MainNetBeta_RPC)
	wsClient, err := ws.Connect(context.Background(), rpc.MainNetBeta_WS)
	if err != nil {
		panic(err)
	}

	wallet, err := solana.PrivateKeyFromSolanaKeygenFile(envWallet)
	if err != nil {
		panic(err)
	}
	fmt.Println("wallet public key:", wallet.PublicKey().String())

	// Get the best routes from Jupiter's Swap API
	quoteUrl, err := url.Parse("https://quote-api.jup.ag")
	if err != nil {
		panic(err)
	}

	quoteUrl.Path += "/v1/quote"

	params := url.Values{}
	params.Add("inputMint", "So11111111111111111111111111111111111111112")
	params.Add("outputMint", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	params.Add("amount", "1000")
	params.Add("slippage", "0.5")
	quoteUrl.RawQuery = params.Encode()
	fmt.Printf("Encoded URL is %q\n", quoteUrl.String())

	resp, err := http.Get(quoteUrl.String())
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	quote := Quote{}
	err = json.NewDecoder(resp.Body).Decode(&quote)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", quote)

	// Get the serialized transaction(s) from Jupiter's Swap API
	swapUrl := "https://quote-api.jup.ag/v1/swap"

	swapReq := SwapRequest{}
	swapReq.Route = quote.Routes[0]
	swapReq.UserPublicKey = wallet.PublicKey().String()

	var swapJsonBody bytes.Buffer
	err = json.NewEncoder(&swapJsonBody).Encode(&swapReq)
	if err != nil {
		panic(err)
	}

	resp, err = http.Post(swapUrl, "application/json", &swapJsonBody)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Println("swapResp:", resp.Body)

	swapResp := SwapResponse{}
	err = json.NewDecoder(resp.Body).Decode(&swapResp)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", swapResp)

	transactionBuffer, err := swapResp.Decode()
	if err != nil {
		panic(err)
	}

	println("transaction count:", len(transactionBuffer))

	for i, swapTx := range transactionBuffer {
		recentBlockhash, err := rpcClient.GetRecentBlockhash(context.TODO(), rpc.CommitmentFinalized)
		if err != nil {
			panic(err)
		}
		swapTx.Message.RecentBlockhash = recentBlockhash.Value.Blockhash

		// The serialized tx coming from Jupiter doesn't yet have a valid signature.
		swapTx.Signatures = []solana.Signature{}
		_, err = swapTx.Sign(
			func(key solana.PublicKey) *solana.PrivateKey {
				if wallet.PublicKey().Equals(key) {
					return &wallet
				}
				return nil
			},
		)
		if err != nil {
			panic(err)
		}

		sig, err := confirm.SendAndConfirmTransaction(
			context.TODO(),
			rpcClient,
			wsClient,
			&swapTx,
		)
		if err != nil {
			panic(err)
		}

		fmt.Println("tx signature:", i+1, sig.String())
	}
}

func (sr *SwapResponse) Decode() (txs []solana.Transaction, err error) {
	var transactionBuffer []solana.Transaction
	if sr.SetupTransaction != "" {
		tx, err := deserializeTx(sr.SetupTransaction)
		if err != nil {
			return nil, err
		}
		transactionBuffer = append(transactionBuffer, *tx)
	}
	if sr.SwapTransaction != "" {
		tx, err := deserializeTx(sr.SwapTransaction)
		if err != nil {
			return nil, err
		}
		transactionBuffer = append(transactionBuffer, *tx)
	}
	if sr.CleanupTransaction != "" {
		tx, err := deserializeTx(sr.CleanupTransaction)
		if err != nil {
			return nil, err
		}
		transactionBuffer = append(transactionBuffer, *tx)
	}
	return transactionBuffer, nil
}

func deserializeTx(base64Tx string) (tx *solana.Transaction, err error) {
	swapTxRaw, err := base64.StdEncoding.DecodeString(base64Tx)
	if err != nil {
		return nil, err
	}

	return solana.MustTransactionFromDecoder(bin.NewBinDecoder(swapTxRaw)), nil
}
