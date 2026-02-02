import pandas as pd
import matplotlib.pyplot as plt
import seaborn as sns
import numpy as np
import sys
import os

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 visualize.py <csv_file>")
        sys.exit(1)

    file_path = sys.argv[1]
    if not os.path.exists(file_path):
        print(f"Error: File {file_path} not found.")
        sys.exit(1)

    print(f"Loading {file_path}...")
    try:
        df = pd.read_csv(file_path)
    except Exception as e:
        print(f"Error reading CSV: {e}")
        sys.exit(1)
        
    print(f"Loaded {len(df):,} rows.")

    # --- Basic Stats ---
    total_updates = df['UpdateCount'].sum()
    total_accounts = len(df)
    mean_updates = df['UpdateCount'].mean()
    median_updates = df['UpdateCount'].median()
    max_updates = df['UpdateCount'].max()

    print("\n--- Statistics ---")
    print(f"Total Accounts: {total_accounts:,}")
    print(f"Total Updates: {total_updates:,}")
    print(f"Mean Updates/Account: {mean_updates:.2f}")
    print(f"Median Updates/Account: {median_updates:.2f}")
    print(f"Max Updates: {max_updates:,}")

    print("\nPercentiles:")
    for p in [90, 95, 99, 99.9, 99.99]:
        val = np.percentile(df['UpdateCount'], p)
        print(f"{p}th percentile: {val:.0f}")

    # Set style
    sns.set_theme(style="whitegrid")
    output_dir = os.path.dirname(file_path) or "."
    base_name = os.path.splitext(os.path.basename(file_path))[0]

    # --- Plot 1: Top 20 Accounts ---
    print("\nGenerating Top 20 Accounts plot...")
    top_20 = df.nlargest(20, 'UpdateCount').copy() # Ensure copy to avoid SettingWithCopyWarning
    # Truncate addresses
    top_20['ShortAddress'] = top_20['Address'].apply(lambda x: x[:6] + "..." + x[-4:] if isinstance(x, str) else str(x))

    plt.figure(figsize=(12, 8))
    sns.barplot(x='UpdateCount', y='ShortAddress', data=top_20, palette='viridis', hue='ShortAddress', legend=False)
    plt.title(f'Top 20 Most Updated Accounts ({base_name})')
    plt.xlabel('Number of Updates')
    plt.ylabel('Account Address')
    plt.tight_layout()
    plot_path = os.path.join(output_dir, f'{base_name}_top_20_accounts.png')
    plt.savefig(plot_path)
    plt.close()
    print(f"Saved {plot_path}")

    # --- Plot 2: Distribution (Log-Log) ---
    print("Generating Update Distribution plot...")
    plt.figure(figsize=(12, 6))
    sns.histplot(df['UpdateCount'], bins=100, log_scale=True)
    plt.title(f'Distribution of Update Counts ({base_name})')
    plt.xlabel('Number of Updates (Log Scale)')
    plt.ylabel('Frequency (Log Scale)')
    plt.tight_layout()
    plot_path = os.path.join(output_dir, f'{base_name}_update_distribution_loglog.png')
    plt.savefig(plot_path)
    plt.close()
    print(f"Saved {plot_path}")

    # --- Plot 3: Lorenz Curve ---
    print("Generating Lorenz Curve...")
    sorted_df = df.sort_values('UpdateCount', ascending=True)
    cum_accounts = np.arange(1, len(sorted_df) + 1) / len(sorted_df)
    cum_updates = sorted_df['UpdateCount'].cumsum() / sorted_df['UpdateCount'].sum()
    
    # Updated for NumPy 2.0 compatibility (trapz removed)
    if hasattr(np, 'trapezoid'):
        gini = 1 - 2 * np.trapezoid(cum_updates, cum_accounts)
    else:
        gini = 1 - 2 * np.trapz(cum_updates, cum_accounts)

    plt.figure(figsize=(8, 8))
    plt.plot(cum_accounts, cum_updates, label=f'Lorenz Curve (Gini: {gini:.4f})')
    plt.plot([0, 1], [0, 1], 'r--', label='Perfect Equality')
    plt.title(f'Lorenz Curve of Account Updates\nGini Coefficient: {gini:.4f}\n({base_name})')
    plt.xlabel('Cumulative Fraction of Accounts')
    plt.ylabel('Cumulative Fraction of Updates')
    plt.legend()
    plt.grid(True)
    plt.tight_layout()
    plot_path = os.path.join(output_dir, f'{base_name}_lorenz_curve.png')
    plt.savefig(plot_path)
    plt.close()
    print(f"Saved {plot_path}")

    print("\nVisualization complete.")

if __name__ == "__main__":
    main()
