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
use spl_token::{
    amount_to_ui_amount, 
    ui_amount_to_amount,
    native_mint
};
use std::{fs::File, time::Duration};
use std::collections::HashMap;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let _mint_addresses = HashMap::from([
        ("SOL", "So11111111111111111111111111111111111111112"),
        ("USDC", "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"),
        ("mSOL", "mSoLzYCxHdYgdzU16g5QSh3i5K3z3KZK7ytfqcJm7So"),
    ]);
    let delay_timer = DelayTimerBuilder::default().build();
    let keypair = read_keypair_file(
        "/home/jay/.config/solana/id.json"
    )?;
    let keypair_buf = keypair.to_bytes();
    let path = "./config.json";
    let file = File::open(path)?;
    let dca: DcaJobs = serde_json::from_reader(file)?;
    let jobs = dca.jobs;
    let mut i = 0;

    for job in jobs {
        println!("in: {} out: {} amt: {}", job.input_mint, job.output_mint, job.amount);
        let mut task_builder = TaskBuilder::default();
        
        let body = move || {

            let kp_buf =  keypair_buf.clone();
            let kp = Keypair::from_bytes(&kp_buf).unwrap();

            let usdc_mint = Pubkey::try_from("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v").unwrap();
            let _msol_mint = Pubkey::try_from("mSoLzYCxHdYgdzU16g5QSh3i5K3z3KZK7ytfqcJm7So").unwrap();
            let sol_mint = Pubkey::try_from("So11111111111111111111111111111111111111112").unwrap();
           let ui_amount = job.amount.clone();

            async move {
                let _ = swap(
                    usdc_mint,
                    sol_mint,
                    ui_amount,
                    1.0,
                    false,
                    kp,
                )
                .await;
            }
        };

        println!("Building task {}", i + 1);

        let task = task_builder
            .set_task_id(i.try_into().unwrap())
            .set_frequency_repeated_by_seconds(60)
            .spawn_async_routine(body)?;

        delay_timer.add_task(task)?;
        i = i + 1;
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
    only_direct_routes: bool,
    keypair: Keypair,
) -> Result<(), Box<dyn std::error::Error>> {
    let rpc_client = RpcClient::new_with_commitment(
        "https://ssc-dao.genesysgo.net/".into(),
        CommitmentConfig::confirmed(),
    );

    let sol_balance = amount_to_ui_amount(
        rpc_client.get_balance(
            &keypair.pubkey()
        )
        .await
        .expect("error getting sol balance"), 
        native_mint::DECIMALS 
    );
    println!("Pre-swap SOL balance: {}", sol_balance);

    let out_token_address = 
        spl_associated_token_account::get_associated_token_address(
            &keypair.pubkey(), 
            &output_mint
        );
    let out_token_acc = rpc_client.get_token_account(
        &out_token_address
    )
    .await
    .expect("err get_token_acc")
    .unwrap();    
    let out_decimals = out_token_acc.token_amount.decimals;
    let out_bal = out_token_acc.token_amount.ui_amount.unwrap();
    println!("Pre-swap output token balance: {}", out_bal);

    let in_token_address =
        spl_associated_token_account::get_associated_token_address(
            &keypair.pubkey(), 
            &input_mint
        );
    let in_token_acc = rpc_client.get_token_account(
        &in_token_address
    )
    .await
    .expect("err get_token_acc")
    .unwrap(); 
    let in_decimals = in_token_acc.token_amount.decimals;
    let in_bal= in_token_acc.token_amount.ui_amount.unwrap();
    println!("Pre-swap USDC balance: {}", in_bal);
    
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
    } = jup_ag::swap(
        quote.clone(), 
        keypair.pubkey()
    )
    .await
    .expect("error getting swap");

    let transactions = [setup, Some(swap), cleanup]
        .into_iter()
        .flatten()
        .collect::<Vec<_>>();
    println!("\nTransactions to send: {}", transactions.len());

    for (i, mut transaction) in transactions
        .into_iter()
        .enumerate() {
        transaction.message.recent_blockhash = rpc_client
            .get_latest_blockhash()
            .await
            .expect("error get_latest_blockhash");
        transaction.sign(
            &[&keypair], 
            transaction.message.recent_blockhash
        );
        println!(
            "Sending transaction {}: {}",
            i + 1,
            transaction.signatures[0]
        );
        let signature = rpc_client
            .send_and_confirm_transaction_with_spinner(&transaction)
            .await
            .expect("error send_and_confirm_tx");
        println!(
            "TX signature: {}: {}",
            i + 1,
            signature
        )
    }

    println!(
        "Post-swap SOL balance: {}",
        amount_to_ui_amount(
            rpc_client
            .get_balance(&keypair.pubkey())
            .await
            .expect("err post swap sol balance"), 
            out_decimals
        )
    );
    println!(
        "Post-swap USDC balance: {}",
        amount_to_ui_amount(
            rpc_client
            .get_token_account_balance(&in_token_address)
            .await
            .expect("err post out balance")
            .amount
            .parse::<u64>()
            .expect("err parsing post out balance"),
            in_decimals 
        )
    );

    Ok(())
}
