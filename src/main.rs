use anyhow::Result;
use delay_timer::prelude::*;
use itertools::Itertools;
use serde::Deserialize;
use solana_client::nonblocking::rpc_client::RpcClient;
use solana_sdk::{
    commitment_config::CommitmentConfig,
    pubkey::Pubkey,
    signature::{read_keypair_file, Keypair, Signer},
};
use spl_token::{amount_to_ui_amount, ui_amount_to_amount};
use std::{fs::File, time::Duration};
use std::collections::HashMap;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let _mint_addresses = HashMap::from([
        ("SOL", "So11111111111111111111111111111111111111112"),
        ("USDC", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"),
    ]);
    let delay_timer = DelayTimerBuilder::default().build();
    let keypair = read_keypair_file(
        "/home/jay/.config/solana/id.json"
    )?;
    let keypair_buf = keypair.to_bytes();
    let path = "./config.json";
    let file = File::open(path)?;
    let dca: DcaJobs = serde_json::from_reader(file)?;

    for job in dca.jobs {
        println!("in: {} out: {} amt: {}", job.input_mint, job.output_mint, job.amount);
        let mut task_builder = TaskBuilder::default();
        
        let body = move || {

            let kp_buf =  keypair_buf.clone();
            let kp = Keypair::from_bytes(&kp_buf).unwrap();

            let input_mint = Pubkey::try_from("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v").unwrap();
            let output_mint = Pubkey::try_from("So11111111111111111111111111111111111111112").unwrap();
            let ui_amount = job.amount.clone();

            async move {
                let _ = swap(
                    input_mint,
                    output_mint,
                    ui_amount,
                    1.0,
                    kp,
                )
                .await;
            }
        };

        let task = task_builder
            .set_task_id(1)
            .set_frequency_repeated_by_seconds(60)
            .set_maximum_parallel_runnable_num(2)
            .spawn_async_routine(body)?;

        delay_timer.add_task(task)?;
    }

    loop {
        sleep_by_tokio(Duration::from_secs(5)).await;
        println!("5 s have elapsed");
    }
}

#[derive(Deserialize)]
struct DcaJobs {
    jobs: Vec<Job>,
}

#[derive(Deserialize, Clone)]
struct Job {
    input_mint: String,
    output_mint: String,
    amount: f64,
}

async fn swap(
    input_mint: Pubkey,
    output_mint: Pubkey,
    ui_amount: f64,
    slippage: f64,
    keypair: Keypair,
) -> Result<(), Box<dyn std::error::Error>> {
    let _msol = Pubkey::try_from("mSoLzYCxHdYgdzU16g5QSh3i5K3z3KZK7ytfqcJm7So")?;

    let rpc_client = RpcClient::new_with_commitment(
        "https://solana-api.projectserum.com".into(),
        CommitmentConfig::confirmed(),
    );

    println!(
        "Pre-swap SOL balance: {}",
        amount_to_ui_amount(
            rpc_client.get_balance(&keypair.pubkey()).await?, 
            9 
        )
    );

    let out_token_address = 
        spl_associated_token_account::get_associated_token_address(
        
            &keypair.pubkey(), 
            &output_mint
        );
    let out_ui_token = rpc_client.get_token_account(&out_token_address).await.expect("err get_token_acc").unwrap();    
    let out_decimals = out_ui_token.token_amount.decimals;
    let out_bal = out_ui_token.token_amount.ui_amount.unwrap();
    println!("Pre-swap output token balance: {}", out_bal);

    let in_token_address =
        spl_associated_token_account::get_associated_token_address(
            &keypair.pubkey(), 
            &input_mint
        );
    let in_ui_token = rpc_client.get_token_account(&in_token_address).await.expect("err get_token_acc").unwrap(); 
    let in_decimals = in_ui_token.token_amount.decimals;
    let in_bal= in_ui_token.token_amount.ui_amount.unwrap();
    println!("Pre-swap USDC balance: {}", in_bal);
    
    let only_direct_routes = false;
    let quotes = jup_ag::quote(
        input_mint,
        output_mint,
        ui_amount_to_amount(ui_amount, in_decimals),
        only_direct_routes,
        Some(slippage),
        None,
    )
    .await
    .expect("error getting quote")
    .data;

    let quote = quotes.get(0).ok_or("No quotes found for SOL to USDC")?;

    println!("Received {} quotes:", quotes.len());
    println!();

    let route = quote
        .market_infos
        .iter()
        .map(|market_info| market_info.label.clone())
        .join(", ");

    println!(
        "Quote: {} USDC for {} SOL via {} (worst case with slippage: {}). Impact: {:.2}%",
        amount_to_ui_amount(quote.in_amount, in_decimals),
        amount_to_ui_amount(quote.out_amount, out_decimals),
        route,
        amount_to_ui_amount(quote.out_amount_with_slippage, out_decimals),
        quote.price_impact_pct * 100.
    );

    let jup_ag::Swap {
        setup,
        swap,
        cleanup,
    } = jup_ag::swap(quote.clone(), keypair.pubkey()).await.expect("error getting swap");

    let transactions = [setup, Some(swap), cleanup]
        .into_iter()
        .flatten()
        .collect::<Vec<_>>();
    println!("\nTransactions to send: {}", transactions.len());

    for (i, mut transaction) in transactions.into_iter().enumerate() {
        transaction.message.recent_blockhash = rpc_client.get_latest_blockhash().await.expect("error get_latest_blockhash");
        transaction.sign(&[&keypair], transaction.message.recent_blockhash);
        println!(
            "Sending transaction {}: {}",
            i + 1,
            transaction.signatures[0]
        );
        let signature = rpc_client
            .send_and_confirm_transaction_with_spinner(&transaction)
            .await.expect("error send_and_confirm_tx");
        println!(
            "TX signature: {}: {}",
            i + 1,
            signature
        )
    }

    println!(
        "Post-swap SOL balance: {}",
        amount_to_ui_amount(rpc_client.get_balance(&keypair.pubkey()).await?, out_decimals)
    );
    println!(
        "Post-swap USDC balance: {}",
        amount_to_ui_amount(
            rpc_client
                .get_token_account_balance(&in_token_address)
                .await?
                .amount
                .parse::<u64>()?,
            in_decimals 
        )
    );

    Ok(())
}
