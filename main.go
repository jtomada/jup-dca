package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Routes struct {
	Routes []Route
}

type Route struct {
	InAmount              float64
	OutAmount             float64
	OutAmountWithSlippage float64
	PriceImpactPct        float64
	MarketInfos           []MarketInfo
}

type MarketInfo struct {
	Id                 string
	Label              string
	InputMint          string
	OutputMint         string
	NotEnoughLiquidity bool
	InAmount           float64
	OutAmount          float64
	PriceImpactPct     float64
	LpFee              Fee
	PlatformFee        Fee
}

type Fee struct {
	Amount float64
	Mint   string
	Pct    float64
}

func main() {
	fmt.Println("Hello Jupiter!")

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

	var j interface{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s", j)
}
