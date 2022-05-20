# jup-dca
A trading bot that automates swaps on the Solana blockchain. Useful for implementing a simple dollar-cost-averaging strategy. 

## Disclaimer
This project was created for educational purposes. Use at your own risk!

## Setup
1. Set up a `.env` file in root with a Solana keypair file as `WALLET_PRIVATE_KEY`, for example `WALLET_PRIVATE_KEY=/home/user/.config/solana/id.json`
2. Set up a `config.json` file in root to represent the DCA jobs to schedule. For example:
```
{
    "jobs": [
        {
            "input_mint": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
            "output_mint": "So11111111111111111111111111111111111111112",
            "amount": 0.0010,
            "cron": "0 */1 * ? * *"
        },
        {
            "input_mint": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
            "output_mint": "mSoLzYCxHdYgdzU16g5QSh3i5K3z3KZK7ytfqcJm7So",
            "amount": 0.0011,
            "cron": "0 */1 * ? * *"
        },
        {
            "input_mint": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
            "output_mint": "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
            "amount": 0.0012,
            "cron": "0 */1 * ? * *"
        }
    ]
}
```

## Running
```
cargo run
```