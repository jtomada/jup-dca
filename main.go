package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Quote struct {
	Data []struct {
		InAmount              int `json:"inAmount"`
		OutAmount             int `json:"outAmount"`
		OutAmountWithSlippage int `json:"outAmountWithSlippage"`
		PriceImpactPct        int `json:"priceImpactPct"`
		MarketInfos           []struct {
			ID                 string `json:"id"`
			Label              string `json:"label"`
			InputMint          string `json:"inputMint"`
			OutputMint         string `json:"outputMint"`
			NotEnoughLiquidity bool   `json:"notEnoughLiquidity"`
			InAmount           int    `json:"inAmount"`
			OutAmount          int    `json:"outAmount"`
			PriceImpactPct     int    `json:"priceImpactPct"`
			LpFee              struct {
				Amount int     `json:"amount"`
				Mint   string  `json:"mint"`
				Pct    float64 `json:"pct"`
			} `json:"lpFee"`
			PlatformFee struct {
				Amount int     `json:"amount"`
				Mint   string  `json:"mint"`
				Pct    float64 `json:"pct"`
			} `json:"platformFee"`
		} `json:"marketInfos"`
	} `json:"data"`
	TimeTaken float64 `json:"timeTaken"`
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

	j := Quote{}
	err = json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%+v\n", j)
}
