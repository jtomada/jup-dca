package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"

	"github.com/davecgh/go-spew/spew"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/text"

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
	SetupTransaction   string `json:"setupTransaction"`
	SwapTransaction    string `json:"swapTransaction"`
	CleanupTransaction string `json:"cleanupTransaction"`
}

type Quote struct {
	Data      []Route `json:"data"`
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

	// endpoint := rpc.MainNetBeta_RPC
	// rpcclient := rpc.New(endpoint)
	wallet := solana.MustPrivateKeyFromBase58(envWallet)
	fmt.Println("wallet public key:", wallet.PublicKey().String())

	base, err := url.Parse("https://quote-api.jup.ag")
	if err != nil {
		panic(err)
	}

	base.Path += "/v1/quote"

	params := url.Values{}
	params.Add("inputMint", "So11111111111111111111111111111111111111112")
	params.Add("outputMint", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	params.Add("amount", "1")
	base.RawQuery = params.Encode()

	fmt.Printf("Encoded URL is %q\n", base.String())

	resp, err := http.Get(base.String())
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	j := Quote{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", j)

	httpposturl := "https://quote-api.jup.ag/v1/swap"
	sr := SwapRequest{}
	sr.Route = j.Data[0]
	sr.UserPublicKey = wallet.PublicKey().String()
	srj, err := json.Marshal(sr)
	req, err := http.NewRequest("POST", httpposturl, bytes.NewBuffer(srj))
	req.Header.Set("X-Custom-Header", "myvalue")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	postresp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer postresp.Body.Close()

	p := SwapResponse{}
	err = json.NewDecoder(postresp.Body).Decode(&p)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", p)
}

func exampleFromBase64(sertx string) {
	data, err := base64.StdEncoding.DecodeString(sertx)
	if err != nil {
		panic(err)
	}

	// parse transaction:
	tx, err := solana.TransactionFromDecoder(bin.NewBinDecoder(data))
	if err != nil {
		panic(err)
	}

	decodeSystemTransfer(tx)
}

func decodeSystemTransfer(tx *solana.Transaction) {
	spew.Dump(tx)

	// we know that the first instruction of the transaction is a `system` program instruction:
	i0 := tx.Message.Instructions[0]

	// parse a system program instruction:
	inst, err := system.DecodeInstruction(i0.ResolveInstructionAccounts(&tx.Message), i0.Data)
	if err != nil {
		panic(err)
	}
	// inst.Impl contains the specific instruction type (in this case, `inst.Impl` is a `*system.Transfer`)
	spew.Dump(inst)
	if _, ok := inst.Impl.(*system.Transfer); !ok {
		panic("the instruction is not a *system.Transfer")
	}

	// OR
	{
		// There is a more general instruction decoder: `solana.DecodeInstruction`.
		// But before you can use `solana.DecodeInstruction`,
		// you must register a decoder for each program ID beforehand
		// by using `solana.RegisterInstructionDecoder` (all solana-go program clients do it automatically with the default program IDs).
		decodedInstruction, err := solana.DecodeInstruction(
			system.ProgramID,
			i0.ResolveInstructionAccounts(&tx.Message),
			i0.Data,
		)
		if err != nil {
			panic(err)
		}
		spew.Dump(decodedInstruction)

		// decodedInstruction == inst
		if !reflect.DeepEqual(inst, decodedInstruction) {
			panic("they are NOT equal (this would never happen)")
		}

		// To register other (not yet registered decoders), you can add them with
		// `solana.RegisterInstructionDecoder` function.
	}

	{
		// pretty-print whole transaction:
		_, err := tx.EncodeTree(text.NewTreeEncoder(os.Stdout, text.Bold("TEST TRANSACTION")))
		if err != nil {
			panic(err)
		}
	}
}
