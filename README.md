# juno
A trading bot that automates swaps on the Solana blockchain. Useful for implementing a simple dollar-cost-averaging strategy. Uses [Jupiter Swap API](https://docs.jup.ag/jupiter-api/swap-api-for-solana), [solana-go](https://github.com/gagliardetto/solana-go), and cron task scheduling under the hood.

## Environment Variables
Set up a `.env` file in root with a Solana keypair file as `WALLET_PRIVATE_KEY`, for example `WALLET_PRIVATE_KEY=/home/user/.config/solana/id.json`