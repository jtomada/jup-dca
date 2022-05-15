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
	Route Route `json:"route"`
	//WrapUnwrapSOL bool   `json:"wrapUnwrapSOL"`
	//FeeAccount    string `json:"feeAccount"`
	//TokenLedger   string `json:"tokenLedger"`
	UserPublicKey string `json:"userPublicKey"`
}

type SwapResponse struct {
	//SetupTransaction   string `json:"setupTransaction"`
	SwapTransaction string `json:"swapTransaction"`
	//CleanupTransaction string `json:"cleanupTransaction"`
}

type Quote struct {
	Routes    []Route `json:"data"`
	TimeTaken float64 `json:"timeTaken"`
}

type Route struct {
	InAmount              int          `json:"inAmount"`
	OutAmount             int          `json:"outAmount"`
	OutAmountWithSlippage int          `json:"outAmountWithSlippage"`
	PriceImpactPct        int          `json:"priceImpactPct"`
	MarketInfos           []MarketInfo `json:"marketInfos"`
}

type MarketInfo struct {
	ID                 string `json:"id"`
	Label              string `json:"label"`
	InputMint          string `json:"inputMint"`
	OutputMint         string `json:"outputMint"`
	NotEnoughLiquidity bool   `json:"notEnoughLiquidity"`
	InAmount           int    `json:"inAmount"`
	OutAmount          int    `json:"outAmount"`
	PriceImpactPct     int    `json:"priceImpactPct"`
	LpFee              Fee    `json:"lpFee"`
	PlatformFee        Fee    `json:"platformFee"`
}

type Fee struct {
	Amount int     `json:"amount"`
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

	endpoint := rpc.MainNetBeta_RPC
	rpcClient := rpc.New(endpoint)
	wsClient, err := ws.Connect(context.Background(), rpc.MainNetBeta_WS)
	if err != nil {
		panic(err)
	}
	wallet := solana.MustPrivateKeyFromBase58(envWallet)
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
	params.Add("amount", "1")
	params.Add("onlyDirectRoutes", "true")
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

	swapResp := SwapResponse{}
	err = json.NewDecoder(resp.Body).Decode(&swapResp)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", swapResp)

	swapTxRaw, err := base64.StdEncoding.DecodeString(swapResp.SwapTransaction)
	if err != nil {
		panic(err)
	}
	swapTx := solana.MustTransactionFromDecoder(bin.NewBinDecoder(swapTxRaw))

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
		swapTx,
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("tx signature:", sig.String())
}
