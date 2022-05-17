use itertools::Itertools;
use serde::Deserialize;
use solana_client::nonblocking::rpc_client::RpcClient;
use solana_sdk::{
    commitment_config::CommitmentConfig,
    pubkey,
    signature::{read_keypair_file, Keypair, Signer},
};
use spl_token::{amount_to_ui_amount, ui_amount_to_amount};
use std::fs::File;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    //let _ = quote().await?; 
    let _ = swap().await?;

    Ok(())
}

async fn quote() -> jup_ag::Result<()> {
    let sol = pubkey!("So11111111111111111111111111111111111111112");
    let msol = pubkey!("mSoLzYCxHdYgdzU16g5QSh3i5K3z3KZK7ytfqcJm7So");
    let usdc = pubkey!("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v");

    let ui_amount = 1.;

    for (output_token, output_decimals) in [(usdc, 6), (msol, 9), (sol, 9)] {
        let data = jup_ag::price(sol, output_token, ui_amount).await?.data;
        println!(
            "Price for {} {} is {} {}",
            data.amount, data.input_symbol, data.price, data.output_symbol
        );

        let slippage = 1.;
        let only_direct_routes = false;
        let quotes = jup_ag::quote(
            sol,
            output_token,
            ui_amount_to_amount(ui_amount, 9),
            only_direct_routes,
            Some(slippage),
            None,
        )
        .await?
        .data;

        println!("Received {} quotes:", quotes.len());
        for (i, quote) in quotes.into_iter().enumerate() {
            let route = quote
                .market_infos
                .iter()
                .map(|market_info| market_info.label.clone())
                .join(", ");
            println!(
                "{}. {} {} for {} {} via {} (worst case with slippage: {}). Impact: {:.2}%",
                i,
                amount_to_ui_amount(quote.in_amount, 9),
                data.input_symbol,
                amount_to_ui_amount(quote.out_amount, output_decimals),
                data.output_symbol,
                route,
                amount_to_ui_amount(quote.out_amount_with_slippage, output_decimals),
                quote.price_impact_pct * 100.
            );
        }
        println!();
    }
    
    Ok(())
}

#[derive(Deserialize)]
struct DcaJobs {
    jobs: Vec<Job>,
}

#[derive(Deserialize)]
struct Job {
    input_mint: String,
    output_mint: String,
    amount: u32
}

async fn swap() -> Result<(), Box<dyn std::error::Error>> {
    let sol = pubkey!("So11111111111111111111111111111111111111112");
    let sol_decimals = 9;
    let msol = pubkey!("mSoLzYCxHdYgdzU16g5QSh3i5K3z3KZK7ytfqcJm7So");
    let usdc = pubkey!("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v");
    let usdc_decimals = 6;
    let path = "./config.json";
    let file = File::open(path)?;
    let dca: DcaJobs = serde_json::from_reader(file)?;
    for job in dca.jobs {
        println!("in: {} out: {} amt: {}", job.input_mint, job.output_mint, job.amount);
    }
    

    let keypair = read_keypair_file("/home/jay/.config/solana/id.json").unwrap_or_else(|err| {
        println!("------------------------------------------------------------------------------------------------");
        println!("Failed to read `swap_example.json`: {}", err);
        println!();
        println!("An ephemeral keypair will be used instead. For a more realistic example, create a new keypair at");
        println!("that location and fund it with a small amount of SOL.");
        println!("------------------------------------------------------------------------------------------------");
        println!();
        Keypair::new()
    });

    let rpc_client = RpcClient::new_with_commitment(
        "https://solana-api.projectserum.com".into(),
        CommitmentConfig::confirmed(),
    );

    let usdc_token_address =
        spl_associated_token_account::get_associated_token_address(&keypair.pubkey(), &usdc);
    println!(
        "Pre-swap SOL balance: {}",
        amount_to_ui_amount(
            rpc_client.get_balance(&keypair.pubkey()).await?, 
            sol_decimals
        )
    );
    let usdc_bal = amount_to_ui_amount(
            rpc_client
                .get_token_account_balance(&usdc_token_address)
                .await?
                .amount
                .parse::<u64>()?,
            usdc_decimals 
    );
    println!(
        "Pre-swap USDC balance: {}", usdc_bal);

    let slippage = 1.;
    let only_direct_routes = false;
    let quotes = jup_ag::quote(
        usdc,
        sol,
        ui_amount_to_amount(0.01, usdc_decimals),
        only_direct_routes,
        Some(slippage),
        None,
    )
    .await?
    .data;

    let quote = quotes.get(0).ok_or("No quotes found for SOL to USDC")?;

    let route = quote
        .market_infos
        .iter()
        .map(|market_info| market_info.label.clone())
        .join(", ");
    println!(
        "Quote: {} USDC for {} SOL via {} (worst case with slippage: {}). Impact: {:.2}%",
        amount_to_ui_amount(quote.in_amount, usdc_decimals),
        amount_to_ui_amount(quote.out_amount, sol_decimals),
        route,
        amount_to_ui_amount(quote.out_amount_with_slippage, sol_decimals),
        quote.price_impact_pct * 100.
    );

    let jup_ag::Swap {
        setup,
        swap,
        cleanup,
    } = jup_ag::swap(quote.clone(), keypair.pubkey()).await?;

    let transactions = [setup, Some(swap), cleanup]
        .into_iter()
        .flatten()
        .collect::<Vec<_>>();
    println!("\nTransactions to send: {}", transactions.len());

    for (i, mut transaction) in transactions.into_iter().enumerate() {
        let (hash, _) = rpc_client.get_latest_blockhash_with_commitment(rpc_client.commitment()).await?;
        transaction.message.recent_blockhash = hash;
        transaction.sign(&[&keypair], transaction.message.recent_blockhash);
        transaction.verify()?;
        println!(
            "Simulating transaction {}: {}",
            i + 1,
            transaction.signatures[0]
        );
        let response = rpc_client.simulate_transaction(&transaction).await?;
        println!("  {:#?}", response.value);
        println!(
            "Sending transaction {}: {}",
            i + 1,
            transaction.signatures[0]
        );
        let _ = rpc_client
            .send_and_confirm_transaction_with_spinner(&transaction)
            .await?;
    }

    println!(
        "Post-swap SOL balance: {}",
        amount_to_ui_amount(rpc_client.get_balance(&keypair.pubkey()).await?, sol_decimals)
    );
    println!(
        "Post-swap USDC balance: {}",
        amount_to_ui_amount(
            rpc_client
                .get_token_account_balance(&usdc_token_address)
                .await?
                .amount
                .parse::<u64>()?,
            usdc_decimals 
        )
    );

    Ok(())
}
