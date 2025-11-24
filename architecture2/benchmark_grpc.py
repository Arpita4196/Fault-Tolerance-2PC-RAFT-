import time
import statistics
import concurrent.futures
import grpc
import matplotlib.pyplot as plt
import proto.telemetry_pb2 as telemetry_pb2
import proto.telemetry_pb2_grpc as telemetry_pb2_grpc

# ------------------------------------
# Configuration
# ------------------------------------
TARGET = "localhost:50052"   # QueryAPI gRPC endpoint
NUM_REQUESTS = 200
CONCURRENCY = 20

# ------------------------------------
# Worker Function
# ------------------------------------
def query_data(_):
    start = time.time()
    try:
        with grpc.insecure_channel(TARGET) as channel:
            stub = telemetry_pb2_grpc.QueryAPIStub(channel)
            now = int(time.time() * 1000)
            request = telemetry_pb2.QueryRequest(
                sensor_id="sensor-001",
                start_ms=now - 60_000,
                end_ms=now
            )
            stub.Range(request)
        return time.time() - start
    except Exception as e:
        print("Error:", e)
        return None

# ------------------------------------
# Run Benchmark
# ------------------------------------
def run_test():
    latencies = []
    start_time = time.time()

    with concurrent.futures.ThreadPoolExecutor(max_workers=CONCURRENCY) as executor:
        for latency in executor.map(query_data, range(NUM_REQUESTS)):
            if latency is not None:
                latencies.append(latency)

    duration = time.time() - start_time
    throughput = len(latencies) / duration

    # Metrics
    avg_latency = statistics.mean(latencies) * 1000
    p90 = statistics.quantiles(latencies, n=10)[8] * 1000
    p99 = max(latencies) * 1000

    # Console Output
    print(f"\n✅ Completed {len(latencies)} requests")
    print(f"Throughput: {throughput:.2f} requests/sec")
    print(f"Average latency: {avg_latency:.2f} ms")
    print(f"P90 latency: {p90:.2f} ms")
    print(f"P99 latency: {p99:.2f} ms")

    # Save results as CSV
    with open("results.csv", "w") as f:
        f.write("Metric,Value\n")
        f.write(f"Throughput,{throughput:.2f}\n")
        f.write(f"Average Latency (ms),{avg_latency:.2f}\n")
        f.write(f"P90 Latency (ms),{p90:.2f}\n")
        f.write(f"P99 Latency (ms),{p99:.2f}\n")

    # ------------------------------------
    # Plot: Count vs Latency
    # ------------------------------------
    plt.figure(figsize=(8, 5))
    plt.hist([l * 1000 for l in latencies], bins=20, color="steelblue", edgecolor="black")
    plt.xlabel("Latency (ms)")
    plt.ylabel("Count")
    plt.title("Count vs Latency for 200 gRPC Requests")
    plt.grid(axis='y', linestyle='--', alpha=0.7)
    plt.tight_layout()
    plt.savefig("latency_hist.png", dpi=300)
    print("📊 Saved histogram as latency_hist.png")

if __name__ == "__main__":
    run_test()

