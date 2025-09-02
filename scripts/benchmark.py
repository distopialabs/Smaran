#!/usr/bin/env python3
"""
Samurai Benchmark Script
This script runs the program with different configurations and logs performance metrics
"""

import subprocess
import time
import os
import csv
import json
from pathlib import Path
from typing import List, Dict, Any
import argparse

class BenchmarkRunner:
    def __init__(self):
        self.accounts = [1, 10, 100, 1000]
        self.blocks = [1000, 5000, 10000]
        self.concurrency = [1, 16, 32, 64]
        
        self.results = []
        self.output_file = "benchmark_results.csv"
        self.log_file = "benchmark.log"
        
    def cleanup_storage(self):
        """Remove storage directory if it exists"""
        storage_path = Path("storage")
        if storage_path.exists():
            import shutil
            shutil.rmtree(storage_path)
            print(f"🧹 Cleaned up storage directory")
    
    def get_directory_size(self, path: str) -> int:
        """Calculate directory size in bytes"""
        total_size = 0
        path_obj = Path(path)
        
        if not path_obj.exists():
            return 0
            
        for file_path in path_obj.rglob('*'):
            if file_path.is_file():
                total_size += file_path.stat().st_size
                
        return total_size
    
    def format_bytes(self, bytes_value: int) -> str:
        """Convert bytes to human readable format"""
        for unit in ['B', 'KB', 'MB', 'GB']:
            if bytes_value < 1024.0:
                return f"{bytes_value:.2f} {unit}"
            bytes_value /= 1024.0
        return f"{bytes_value:.2f} TB"
    
    def build_program(self) -> bool:
        """Build the Go program"""
        print("🔨 Building program...")
        try:
            result = subprocess.run(
                ["go", "build", "-o", "samurai", "."],
                capture_output=True,
                text=True,
                check=True
            )
            print("✅ Program built successfully")
            return True
        except subprocess.CalledProcessError as e:
            print(f"❌ Failed to build program: {e}")
            print(f"Error output: {e.stderr}")
            return False
    
    def run_benchmark(self, accounts: int, blocks: int, concurrency: int) -> Dict[str, Any]:
        """Run a single benchmark"""
        print(f"\n🚀 Running benchmark: Accounts={accounts}, Blocks={blocks}, Concurrency={concurrency}")
        
        # Clean up before each run
        self.cleanup_storage()
        
        # Start timing
        start_time = time.time()
        
        try:
            # Run the program
            cmd = ["./samurai", "-b", str(blocks), "-a", str(accounts), "-c", str(concurrency)]
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=3600  # 1 hour timeout
            )
            
            end_time = time.time()
            duration_ms = int((end_time - start_time) * 1000)
            
            if result.returncode == 0:
                # Calculate storage size
                storage_size = self.get_directory_size("storage")
                storage_formatted = self.format_bytes(storage_size)
                
                print(f"✅ Benchmark completed in {duration_ms}ms, Storage: {storage_formatted}")
                
                return {
                    "accounts": accounts,
                    "blocks": blocks,
                    "concurrency": concurrency,
                    "time_taken_ms": duration_ms,
                    "storage_size_bytes": storage_size,
                    "status": "SUCCESS",
                    "output": result.stdout,
                    "error": None
                }
            else:
                print(f"❌ Benchmark failed after {duration_ms}ms")
                print(f"Error output: {result.stderr}")
                
                return {
                    "accounts": accounts,
                    "blocks": blocks,
                    "concurrency": concurrency,
                    "time_taken_ms": duration_ms,
                    "storage_size_bytes": 0,
                    "status": "FAILED",
                    "output": result.stdout,
                    "error": result.stderr
                }
                
        except subprocess.TimeoutExpired:
            end_time = time.time()
            duration_ms = int((end_time - start_time) * 1000)
            print(f"⏰ Benchmark timed out after {duration_ms}ms")
            
            return {
                "accounts": accounts,
                "blocks": blocks,
                "concurrency": concurrency,
                "time_taken_ms": duration_ms,
                "storage_size_bytes": 0,
                "status": "TIMEOUT",
                "output": "",
                "error": "Execution timed out after 1 hour"
            }
            
        except Exception as e:
            end_time = time.time()
            duration_ms = int((end_time - start_time) * 1000)
            print(f"💥 Benchmark crashed after {duration_ms}ms: {e}")
            
            return {
                "accounts": accounts,
                "blocks": blocks,
                "concurrency": concurrency,
                "time_taken_ms": duration_ms,
                "storage_size_bytes": 0,
                "status": "CRASHED",
                "output": "",
                "error": str(e)
            }
    
    def write_results_to_csv(self):
        """Write results to CSV file"""
        with open(self.output_file, 'w', newline='') as csvfile:
            fieldnames = ['Accounts', 'Blocks', 'Concurrency', 'TimeTaken(ms)', 'StorageSize(bytes)', 'Status']
            writer = csv.DictWriter(csvfile, fieldnames=fieldnames)
            
            writer.writeheader()
            for result in self.results:
                writer.writerow({
                    'Accounts': result['accounts'],
                    'Blocks': result['blocks'],
                    'Concurrency': result['concurrency'],
                    'TimeTaken(ms)': result['time_taken_ms'],
                    'StorageSize(bytes)': result['storage_size_bytes'],
                    'Status': result['status']
                })
    
    def write_detailed_log(self):
        """Write detailed log to file"""
        with open(self.log_file, 'w') as logfile:
            logfile.write(f"Samurai Benchmark Log - {time.strftime('%Y-%m-%d %H:%M:%S')}\n")
            logfile.write("=" * 50 + "\n\n")
            
            for result in self.results:
                logfile.write(f"=== Benchmark: Accounts={result['accounts']}, Blocks={result['blocks']}, Concurrency={result['concurrency']} ===\n")
                logfile.write(f"Status: {result['status']}\n")
                logfile.write(f"Time: {result['time_taken_ms']}ms\n")
                logfile.write(f"Storage: {self.format_bytes(result['storage_size_bytes'])}\n")
                
                if result['error']:
                    logfile.write(f"Error: {result['error']}\n")
                
                if result['output']:
                    logfile.write("Output:\n")
                    logfile.write(result['output'])
                
                logfile.write("\n" + "=" * 50 + "\n\n")
    
    def print_summary(self):
        """Print benchmark summary"""
        print("\n🎯 Benchmark Summary")
        print("=" * 50)
        
        total_runs = len(self.results)
        successful_runs = sum(1 for r in self.results if r['status'] == 'SUCCESS')
        failed_runs = total_runs - successful_runs
        
        print(f"📊 Total runs: {total_runs}")
        print(f"✅ Successful: {successful_runs}")
        print(f"❌ Failed: {failed_runs}")
        
        if successful_runs > 0:
            # Calculate averages
            successful_results = [r for r in self.results if r['status'] == 'SUCCESS']
            avg_time = sum(r['time_taken_ms'] for r in successful_results) / len(successful_results)
            avg_storage = sum(r['storage_size_bytes'] for r in successful_results) / len(successful_results)
            
            print(f"⏱️  Average time: {avg_time:.0f}ms")
            print(f"💾 Average storage: {self.format_bytes(int(avg_storage))}")
            
            # Find fastest and slowest
            fastest = min(successful_results, key=lambda x: x['time_taken_ms'])
            slowest = max(successful_results, key=lambda x: x['time_taken_ms'])
            
            print(f"\n🚀 Fastest run: Accounts={fastest['accounts']}, Blocks={fastest['blocks']}, Concurrency={fastest['concurrency']} ({fastest['time_taken_ms']}ms)")
            print(f"🐌 Slowest run: Accounts={slowest['accounts']}, Blocks={slowest['blocks']}, Concurrency={slowest['concurrency']} ({slowest['time_taken_ms']}ms)")
        
        print(f"\n📈 Results saved to: {self.output_file}")
        print(f"📝 Detailed logs saved to: {self.log_file}")
    
    def run_all_benchmarks(self):
        """Run all benchmark combinations"""
        print("🚀 Starting Samurai Benchmark Suite")
        print("=" * 50)
        print(f"Testing configurations:")
        print(f"  Accounts: {self.accounts}")
        print(f"  Blocks: {self.blocks}")
        print(f"  Concurrency: {self.concurrency}")
        print(f"Total combinations: {len(self.accounts) * len(self.blocks) * len(self.concurrency)}")
        print()
        
        # Check prerequisites
        if not os.path.exists("main.go"):
            print("❌ main.go not found. Please run this script from the project root directory.")
            return False
        
        # Build the program
        if not self.build_program():
            return False
        
        # Run all benchmarks
        total_combinations = len(self.accounts) * len(self.blocks) * len(self.concurrency)
        current_run = 0
        
        for accounts in self.accounts:
            for blocks in self.blocks:
                for concurrency in self.concurrency:
                    current_run += 1
                    print(f"\n--- Progress: {current_run}/{total_combinations} ---")
                    
                    result = self.run_benchmark(accounts, blocks, concurrency)
                    self.results.append(result)
                    
                    # Small delay between runs
                    time.sleep(1)
        
        # Write results
        self.write_results_to_csv()
        self.write_detailed_log()
        
        # Print summary
        self.print_summary()
        
        return True

def main():
    parser = argparse.ArgumentParser(description='Run Samurai benchmarks')
    parser.add_argument('--output', default='benchmark_results.csv', help='Output CSV file')
    parser.add_argument('--log', default='benchmark.log', help='Log file')
    
    args = parser.parse_args()
    
    runner = BenchmarkRunner()
    runner.output_file = args.output
    runner.log_file = args.log
    
    success = runner.run_all_benchmarks()
    
    if success:
        print("\n🎉 All benchmarks completed!")
        return 0
    else:
        print("\n💥 Benchmark suite failed!")
        return 1

if __name__ == "__main__":
    exit(main())
