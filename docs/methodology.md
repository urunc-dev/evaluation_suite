# Benchmarking Methodology Doc

> This is a live document, I will be updating it dependig on feedback. 
>    Furthermore, the Memory and CPU methodologies are most likely to change forexample when cgroups is merged to urunc. However for now thats what i have come up with.

## 1. Objective

This evaluation will compare the performance and host-resource cost of `runc`, gVisor, Kata Containers, and `urunc`. Native Linux execution will be used as the performance baseline where an equivalent native workload can be run. `runc` will be used as the OCI-runtime baseline, particularly for lifecycle tests where a native equivalent does not exist.

The study will answer the following questions:

1. What latency does each runtime add to OCI create, start, application readiness, and delete operations?
2. What CPU overhead does each sandbox add for compute-heavy, syscall-heavy, and synchronization-heavy workloads?
3. How much memory is assigned, committed, and consumed by the workload, runtime, VMM, and complete host?
4. How do the runtimes affect storage throughput, IOPS, and operation latency?
5. How do they affect HTTP throughput, request round-trip time, and tail latency?
6. How stable are these results under CPU, memory, and I/O constraints?

This is not intended to produce one universal score. Each result will be reported together with the runtime configuration and isolation path that produced it.

## 2. Metrics Matrix

| Area        | Metrics                                                                                       | Primary tooling                                                         |
| ----------- | --------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Lifecycle   | OCI task create, start, delete, application readiness, HTTP readiness                         | Go containerd client, timed `ctr tasks` commands for validation         |
| Memory      | Guest-assigned memory, payload cgroup usage, sandbox/VMM cost, process PSS, total host impact | Custom `memory-bench`, cgroup v2, `/proc`, `smaps_rollup`, Go collector |
| CPU         | Native-to-sandbox throughput and latency overhead, scaling efficiency, total host CPU cost    | Custom static `cpu-bench`, cgroup v2, `perf stat`                       |
| Storage     | Sequential throughput, random IOPS, latency percentiles, sync-write cost, metadata operations | `fio`, custom metadata mode; `sysbench fileio` as a secondary check     |
| Network     | HTTP throughput, request RTT, p50/p90/p99/p99.9 latency, errors, and raw TCP throughput       | Fortio, `iperf3` for bulk TCP throughput                                |
| Tail/stress | p99/p99.9 behavior, deadline misses, throughput loss, and recovery under resource constraints | Fixed-rate Fortio, cgroup-constrained CPU/memory/I/O runs               |

## 3. Systems and Configuration Variants

The following will be treated as distinct systems under test. Results from different backends will not be pooled.

| Identifier         | Runtime path                                                                                  |
| ------------------ | --------------------------------------------------------------------------------------------- |
| `native`           | Benchmark binary in a host cgroup with the same payload resource limits                       |
| `runc`             | containerd -> runc shim -> process -> host kernel                                             |
| `runsc`            | containerd -> runsc -> Sentry/Gofer as configured -> host kernel                              |
| `kata-qemu`        | containerd -> Kata shim -> QEMU/KVM -> guest kernel -> agent -> workload                      |
| `kata-firecracker` | containerd -> Kata shim -> Firecracker/KVM -> guest kernel -> agent -> workload, if supported |
| `urunc-<monitor>`  | containerd -> urunc -> selected monitor/VMM -> unikernel workload                             |

For every identifier, the harness will store the runtime version, binary checksum, containerd configuration, runtime configuration checksum, hypervisor/monitor version, guest-kernel version, snapshotter, filesystem, and workload image digest.


## 4. Fairness Rules

### 4.1 Same work

For each comparable experiment, every runtime will receive:

- The same algorithm and input data
- The same number of operations or measured duration
- The same thread count
- The same correctness check
- The same static executable where the runtime supports it
- The same compiler, compiler flags, and target instruction set
- The same resource profile
- The same storage dataset and network response payload

The custom C benchmarks will be compiled with a conservative common CPU target, not `-march=native`. The exact compiler version and flags will be recorded. A correctness checksum will be checked after every trial so that a fast but incomplete execution is never accepted.

`urunc` may require different packaging from the standard Linux runtimes.

Only common-core results will be used for direct all-runtime comparisons. Any source changes, linked-library differences, or unikernel-specific configuration will be disclosed.

### 4.2 Two resource-equivalence modes

VM-backed and unikernel runtimes consume resources outside the payload process. A single definition of "equal memory" can therefore hide or unfairly charge this overhead. The experiments will use two separate modes.

| Mode                   | Rule                                                                                                                                                    | Used for                                            |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| Payload-equivalent     | The benchmark payload sees the same vCPU count, memory limit, thread count, and input on every runtime. Extra runtime/VMM cost is measured on the host. | CPU, memory performance, storage, and network       |
| Host-budget-equivalent | The complete sandbox, including supporting processes where controllable, receives the same top-level host CPU and memory budget.                        | Density, efficiency, and constrained-resource tests |

The primary payload-equivalent profile will be **1 vCPU and 1 GiB payload memory**. Scaling experiments will use `1`, `2`, and `4` vCPUs, subject to the physical host capacity. Constrained profiles will include **0.5 vCPU / 512 MiB** and **1 vCPU / 512 MiB**. These values will be frozen after the pilot and will not change between runtimes.

Configured guest memory for Kata or `urunc` is not treated as equivalent to host memory consumed. The configured value, guest-visible value, sandbox cgroup usage, VMM PSS, and total host delta will all be reported separately.

### 4.3 Host and software controls

- All comparable trials will run on the same physical host.
- Runtime order will be randomized in balanced blocks.
- No unrelated user workload will run during a block.
- CPU governor, turbo policy, SMT state, kernel parameters, NUMA placement, and swap state will remain fixed and recorded.
- Benchmark CPUs will be pinned. Host services and the load generator will use reserved CPUs outside the benchmark CPU set where possible.
- Images will be pulled and unpacked before timed tests.
- The same containerd version, snapshotter, CNI configuration, filesystem, mount options, and host network path will be used.
- Logging verbosity and collectors will be identical across runtimes.
- The load-generator machine or reserved host cores must demonstrate capacity above the fastest system under test.

### 4.4 Runtime defaults and tuning

The first comparison will use documented default production configurations. A tuned feature such as VM pooling, snapshot/restore, alternative gVisor platform, or different hypervisor will be evaluated as a separate named configuration. It will never be enabled for only one runtime while still labeling the result as a default comparison.

## 5. Harness Design

The harness will be written primarily in Go. It will read a versioned experiment manifest, prepare a trial, invoke the selected runtime adapter, collect host and workload measurements, validate output, clean up, and write raw results.

<img  height="600" alt="image" src="https://github.com/user-attachments/assets/ba92d75a-c162-4dc1-8e6a-6521d90a4268" />

The runtime adapter will expose the following operations:

```text
prepare -> create task -> start task -> wait ready -> stop -> delete task -> cleanup
```

All elapsed times will use Go's monotonic clock or `clock_gettime(CLOCK_MONOTONIC_RAW)`. UTC wall-clock timestamps will be retained only for log correlation.

### 5.1 Proposed result layout

```text
results/<experiment-id>/<run-id>/
|-- manifest.yaml
|-- environment.json
|-- trials.jsonl
|-- raw/
|   |-- <trial-id>-workload.json
|   |-- <trial-id>-collector.jsonl
|   `-- <trial-id>-tool-output.json
`-- logs/
    |-- <trial-id>-stdout.log
    |-- <trial-id>-stderr.log
    `-- <trial-id>-runtime.log
```

The harness will preserve original tool output. Summaries will be generated from raw data and will never replace it.

For payload-equivalent tests, the adapter will apply an invocation equivalent to the following. The exact command is generated from the manifest so trial directories and ports are unique and are not hard-coded in the benchmark scripts.

```sh
nerdctl run --rm \
  --runtime "$RUNTIME_HANDLER" \
  --cpus "$PAYLOAD_CPUS" \
  --memory "$PAYLOAD_MEMORY" \
  --cpuset-cpus "$PAYLOAD_CPUSET" \
  --name "$TRIAL_ID" \
  "$IMAGE_DIGEST" \
  "$BENCHMARK_COMMAND"
```

The final lifecycle adapter will use the containerd API instead of spawning this command. Native baselines will run in a transient cgroup with the same CPU set, quota, and memory limit.

## 6. Lifecycle Methodology

### 6.1 Workload

The lifecycle experiment will use `nginx:alpine` as the primary workload. This image is small, widely available, starts a long-running server process by default, and provides a simple HTTP readiness target without requiring a custom benchmark program.

The image will be pulled, unpacked, and pinned by digest before timed trials begin. The container will use the image's default command so that the benchmark does not add shell startup overhead through `/bin/sh -c ...`.

HTTP readiness will be measured by probing the nginx HTTP endpoint after the task-start request is issued. If the default `nginx:alpine` image is used unchanged, the probe target will be `/`.

For `urunc`, the nginx image will be packagesd to be compatible with `urunc` using [bunny](https://github.com/nubificus/bunny) as explained in this tutorial [https://urunc.io/tutorials/existing-container-linux/](https://urunc.io/tutorials/existing-container-linux/)

### 6.2 Measurement boundaries

containerd distinguishes a container metadata object from a live task. Creating metadata with `ctr containers create` or the containerd Go client's `NewContainer` is therefore not the OCI runtime create measurement. Container metadata creation, snapshot preparation, image pull, and image unpack will happen before the measured lifecycle interval.

The primary lifecycle measurements will be event-based. The harness will issue lifecycle operations through the containerd Go client, but the recorded OCI lifecycle boundaries will be based on the corresponding containerd task events.

| Metric                    | Start                                                             | End                                       |
| ------------------------- | ----------------------------------------------------------------- | ----------------------------------------- |
| Task create event latency | Immediately before containerd `NewTask` / task-create request     | Matching task-create event is observed    |
| Task start event latency  | Immediately before task-start request                             | Matching task-start event is observed     |
| Task delete event latency | Immediately before task-delete request after the task has stopped | Matching task-delete event is observed    |
| HTTP ready latency        | Immediately before task-start request                             | First successful HTTP response from nginx |

Only these four metrics will be reported as the primary lifecycle results. The containerd Go client will be used so that CLI process startup is not included. Client RPC return times may still be logged as diagnostic data during development, but they will not be part of the primary reported lifecycle metrics.

During harness development, event-based measurements may be validated against equivalent `ctr` operations. These checks will only be used for implementation validation because `ctr` includes CLI overhead and reports client-observed behavior rather than event-observed lifecycle latency.

A runtime adapter layer will be used where necessary. For example, runc and Kata commonly use Runtime v2 handlers such as `io.containerd.runc.v2` and `io.containerd.kata.v2`, while gVisor commonly uses `io.containerd.runsc.v1` with `containerd-shim-runsc-v1`. Because handlers can differ in task event behavior, IO behavior, and accepted container specs, each runtime must pass a preflight lifecycle check before its measurements are accepted.

### 6.3 Lifecycle experiment procedure

1. Verify that the `nginx:alpine` image digest is present locally.
2. Pre-create a unique container and snapshot outside the timed interval.
3. Start the host collector.
4. Subscribe to containerd task events for the trial container ID.
5. Start the task-create timer immediately before issuing the containerd task-create request.
6. Stop the task-create timer when the matching task-create event is observed.
7. Start the task-start and HTTP-ready timers immediately before issuing the task-start request.
8. Stop the task-start timer when the matching task-start event is observed.
9. Probe the nginx HTTP endpoint until the first valid response and record HTTP ready latency.
10. Hold the workload for five seconds to confirm that it remains healthy.
11. Signal termination and wait for the task to exit.
12. Start the task-delete timer immediately before issuing the task-delete request.
13. Stop the task-delete timer when the matching task-delete event is observed.
14. Delete container metadata and snapshot outside the OCI delete measurement.
15. Audit cleanup and retain all errors, including leftover tasks, containers, cgroups, mounts, network interfaces, ports, or runtime helper processes.

The HTTP-readiness test includes application initialization and network availability, so it will be reported separately from OCI task start.

### 6.4 Cold and warm lifecycle runs

- **Runtime-cold:** no live sandbox, no pre-created VM/unikernel pool, and a freshly restarted runtime/containerd block. Images remain locally available. This does not claim that the host page cache is cold.
- **Warm:** image and runtime code paths have been exercised by three untimed trials, with no live sandbox carried into the measured trial.
- **Provisioning-cold:** image absent and pulled during the trial. This is optional and reported separately from runtime latency.

Lifecycle tests will use 10 runtime-cold repetitions and 30 warm repetitions per runtime. Runtime order will be randomized by block. Serial and concurrent launch will be tested separately at concurrency `1`, `2`, `4`, and `8`.

If a runtime cannot reliably produce the required task events through the selected containerd handler, its lifecycle results will not be silently mixed into the main comparison. The failure mode will be reported separately, and the runtime may be measured through a runtime-specific adapter only if that adapter's boundaries are clearly documented.

## 7. CPU Methodology

### 7.1 `cpu-bench` workload

I will build one statically linked C program named `cpu-bench`. It will expose the following modes:

| Mode             | Exact work                                                                     | What it measures                                                              |
| ---------------- | ------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| `prime`          | Count primes in a fixed integer range using the same deterministic algorithm   | Integer compute throughput                                                    |
| `sha256-scalar`  | Repeatedly hash a deterministic in-memory buffer using a scalar implementation | Compute and memory-processing throughput without optional crypto instructions |
| `getpid-raw`     | Repeated `syscall(SYS_getpid)` calls                                           | Raw syscall transition overhead                                               |
| `clock-raw`      | Repeated `syscall(SYS_clock_gettime, ...)` calls                               | Raw time-related syscall overhead                                             |
| `clock-vdso`     | Repeated libc `clock_gettime()` calls                                          | Fast-path/vDSO behavior, reported separately from raw syscall mode            |
| `futex-pingpong` | Two threads alternate through a futex word                                     | Synchronization and scheduler overhead                                        |
| `pipe-pingpong`  | Two threads exchange one-byte tokens through two pipes                         | IPC, syscall, and context-switch overhead                                     |

The binary will accept `--mode`, `--duration`, `--iterations`, `--threads`, `--warmup`, and `--seed`. Before measurement it will initialize deterministic inputs and synchronize all workers on a barrier. It will then measure only the selected operation and emit JSON containing:

- Workload and parameters
- Elapsed monotonic time
- Total and per-thread operation counts
- Operations per second or MiB/s
- Per-operation latency where applicable
- Correctness checksum
- Whether all threads completed

### 7.2 CPU execution plan

1. Run the identical binary natively in a transient host cgroup and through every compatible runtime.
2. Use a five-second in-process warm-up followed by a 30-second measured interval.
3. Run 10 measured repetitions per mode, runtime, and thread count.
4. Test thread counts `1`, `2`, and `4`, without exceeding the allocated CPUs.
5. Pin the payload and identify/pin supporting processes consistently where the runtime permits it.
6. Capture workload cgroup `cpu.stat` fields (`usage_usec`, `user_usec`, `system_usec`, `nr_throttled`, and `throttled_usec`), total supporting-process CPU, host CPU delta, context switches, migrations, page faults, CPU PSI, and `perf stat` counters.

`perf stat` will collect cycles, instructions, branches, branch misses, context switches, migrations, and faults where the runtime and hardware allow them. Unsupported or multiplexed counters will be marked rather than replaced with zero.

### 7.3 CPU results

The primary result will be useful throughput. Cost will be expressed as total host CPU seconds per million operations. I will report:

$$
\text{Throughput ratio}_{runtime} = \frac{\text{throughput}_{runtime}}{\text{throughput}_{native}}
$$

$$
\text{CPU overhead}_{runtime} = \left(\frac{\text{host CPU/op}_{runtime}}{\text{host CPU/op}_{native}} - 1\right) \times 100\%
$$

For lifecycle-style comparisons without a native equivalent, `runc` will replace native in the denominator.

## 8. Memory Methodology

### 8.1 `memory-bench` workload

I will build a statically linked C program named `memory-bench`. It will emit timestamped JSON events named `READY`, `ALLOCATED`, `TOUCHED`, `HOLDING`, `VERIFIED`, and `RELEASED`. The host collector will align its samples with these phases.

Memory will be written with non-zero deterministic data one page at a time. This prevents a large virtual allocation from being mistaken for physically backed memory. A checksum will be verified before release.

| Mode               | Procedure                                                                   | Primary result                                |
| ------------------ | --------------------------------------------------------------------------- | --------------------------------------------- |
| `idle`             | Initialize, emit `READY`, allocate no large buffer, hold for 60 s           | Fixed sandbox memory cost                     |
| `alloc-touch`      | Allocate and touch `64`, `256`, and `512` MiB, then hold each size for 30 s | Host growth for known touched bytes           |
| `incremental`      | Add and touch 64 MiB every 5 s until the selected maximum                   | Incremental memory slope and reclaim behavior |
| `bandwidth-read`   | Repeated sequential reads from a pre-touched buffer for 30 s                | GiB/s and host CPU/GiB                        |
| `bandwidth-write`  | Repeated sequential writes to a pre-touched buffer for 30 s                 | GiB/s and host CPU/GiB                        |
| `bandwidth-copy`   | Repeated copy between two pre-touched buffers for 30 s                      | GiB/s and host CPU/GiB                        |
| `pagefault-seq`    | Map a fixed region and touch one byte per page sequentially                 | Faults/s and ns/page                          |
| `pagefault-random` | Touch pages in a deterministic random permutation                           | Faults/s and ns/page                          |
| `pointer-chase`    | Follow a deterministic randomized pointer chain in a fixed working set      | Memory-access latency                         |
| `limit`            | Increase touched memory toward and beyond a cgroup limit                    | Reclaim, pressure, OOM, and exit behavior     |

### 8.2 Host-side memory collector

The Go collector will discover and track:

- Payload cgroup
- Sandbox/parent cgroup
- containerd shim
- gVisor Sentry and Gofer processes
- Kata VMM and shim processes
- `urunc` monitor/VMM and supporting processes

It will sample inexpensive cgroup and `/proc` counters every 100 ms. PSS from `/proc/<pid>/smaps_rollup` will be sampled every one second because it is more expensive. It will collect:

| Accounting layer    | Values                                                                                        |
| ------------------- | --------------------------------------------------------------------------------------------- |
| Guest configuration | Configured guest RAM, vCPUs, ballooning and sharing settings                                  |
| Guest-visible state | Guest total/free/available memory where collection is supported without changing the workload |
| Payload cgroup      | `memory.current`, `memory.peak`, anonymous/file/slab breakdown, swap, events, PSI             |
| Sandbox cgroup      | Total cgroup consumption including support processes where hierarchy permits                  |
| Host processes      | RSS and PSS for shim, Sentry/Gofer, VMM, monitor, and related processes                       |
| Host                | `MemAvailable`, `/proc/vmstat`, host memory PSI, OOM events                                   |

RSS values will not be summed as the main memory result because shared pages can be counted repeatedly. PSS, cgroup totals, and host deltas will be reported as separate views. I will not add them together if their accounting boundaries overlap.

### 8.3 Memory experiment procedure

1. Capture host and runtime baseline before create.
2. Start the workload and wait for `READY`.
3. Sample the idle phase for 60 seconds.
4. Trigger exactly one selected memory mode.
5. At `TOUCHED`/`HOLDING`, confirm the expected checksum and touched-byte count.
6. Continue collecting through `RELEASED` and deletion.
7. Measure time for host memory to return within 5% of the pre-trial baseline.
8. Mark OOM kills and allocation failures as outcomes, not outliers.

Each regular memory mode will have 10 measured repetitions. Limit/OOM tests will have at least five repetitions and will run inside a host-safe top-level cgroup with a timeout.

### 8.4 Density measurement

The `idle` mode will be launched at instance counts `1`, `2`, `4`, `8`, and successive powers of two until a predeclared stopping condition. An instance counts as healthy only if it emits `READY`, remains alive for 60 seconds, and responds to a validation request.

The run will stop when any of the following occurs:

- More than 1% of instances fail
- Any host OOM kill occurs
- Host memory PSI `full` exceeds the predeclared safety threshold
- Readiness p99 exceeds the timeout
- The host reserve falls below the predeclared safe minimum

The incremental bytes per additional sandbox will be estimated from the slope of total host memory versus healthy instance count, not only from a one-instance RSS snapshot.


## 9. Storage Methodology

### 9.1 Storage paths

Two storage paths will be tested and reported separately:

- **Data volume:** the same host-backed benchmark directory or block volume mounted into each runtime
- **Root filesystem:** the runtime's normal writable root filesystem

The data-volume path is the primary comparison because it uses the same underlying host storage. Root-filesystem results are secondary because snapshotter and image-layout effects are part of that path.

### 9.2 `fio` workload matrix

The pilot default will use a 4 GiB test file, 10-second ramp-up, 60-second measured duration, one job, and JSON+ output. If the test host cannot support 4 GiB safely, the size will be reduced once during the pilot and then frozen for every runtime.

| Test             | `rw`                           | Block size | I/O depth | Purpose                    |
| ---------------- | ------------------------------ | ---------: | --------: | -------------------------- |
| Sequential read  | `read`                         |      1 MiB |  1 and 32 | Streaming read throughput  |
| Sequential write | `write`                        |      1 MiB |  1 and 32 | Streaming write throughput |
| Random read      | `randread`                     |      4 KiB |  1 and 32 | Read IOPS and latency      |
| Random write     | `randwrite`                    |      4 KiB |  1 and 32 | Write IOPS and latency     |
| Mixed random     | `randrw`, 70% read             |      4 KiB |        32 | Database-like mixed I/O    |
| Sync write       | `write` with sync/fsync policy |      4 KiB |         1 | Durability-path latency    |

Representative command template:

```sh
fio \
  --name="$TEST_ID" \
  --filename=/bench/testfile \
  --rw="$RW_MODE" \
  --bs="$BLOCK_SIZE" \
  --iodepth="$IO_DEPTH" \
  --ioengine="$IO_ENGINE" \
  --size=4G \
  --direct=1 \
  --time_based=1 \
  --ramp_time=10 \
  --runtime=60 \
  --group_reporting=1 \
  --output-format=json+
```

The mixed `randrw` profile will additionally set `--rwmixread=70`. Each trial will receive a private benchmark-data directory created by the harness and mounted at `/bench`; tool output will be written to that trial's raw-result directory by the harness.

The chosen I/O engine will be one supported equivalently by the common-core runtimes. Direct I/O support will be verified for every path. If a runtime does not honor or support direct I/O, that result will be marked incompatible; it will not silently fall back to buffered I/O. Buffered and direct tests will be separate experiment profiles.

Read tests will use a prepared file. Preparation, cache handling, and cleanup will occur outside the timed interval. Cache-cold and cache-warm tests will never be averaged together.

### 9.3 Metadata and secondary file I/O

`sysbench fileio` will be used only as a secondary filesystem-level check. It is not a pure metadata benchmark. For metadata overhead, the common static benchmark will execute fixed batches of create, `stat`, rename, and unlink operations on empty files in private per-trial directories. It will report operations/s, p50/p95/p99 operation latency, errors, and a final directory-count validation.

The secondary `sysbench` profile will use the same prepared data size and one thread:

```sh
sysbench fileio \
  --file-total-size=4G \
  --file-test-mode=rndrw \
  --file-io-mode=sync \
  --threads=1 \
  --time=60 \
  --rand-seed="$SEED" \
  run
```

Its `prepare`, `run`, and `cleanup` stages will use a private per-trial directory; only `run` will be timed.

### 9.4 Storage metrics

For each `fio` job I will retain:

- Read/write MiB/s
- Read/write IOPS
- Submission, completion, and total latency
- p50, p95, p99, and p99.9 completion latency
- Workload and total host CPU time
- CPU seconds per GiB or per million I/O operations
- cgroup `io.stat`, physical-device `/proc/diskstats`, and I/O PSI deltas
- Short I/O and error counts

Every storage profile will use 10 measured repetitions. Results will identify the device, filesystem, snapshotter, mount options, free space, cache state, and I/O engine.

## 10. Network Methodology

### 10.1 Server and topology

I will use a standard nginx webserver. In the case of `urunc`, I will repackage it into a runnable image that `urunc` can run.

Fortio will run either on a dedicated peer or in a fixed host-network container pinned to reserved host CPUs.

The primary path will be host/peer -> runtime network interface -> benchmark server. Loopback and cross-instance paths, if added, will be separate topologies.

The server will be launched through the runtime adapter with the common payload limits. An equivalent exploratory command is:

```sh
nerdctl run --rm --detach \
  --runtime "$RUNTIME_HANDLER" \
  --cpus 1 \
  --memory 1G \
  --cpuset-cpus "$PAYLOAD_CPUSET" \
  --publish "$HOST_PORT:8080" \
  --name "$TRIAL_ID" \
  "$HTTP_BENCH_IMAGE_DIGEST" \
  /http-bench-server --listen=:8080 --workers=1 --payload-bytes=1024
```

The published-port/CNI path is part of this end-to-end network test and will be held constant. A host-network result, if collected, will be labeled as a different topology.

### 10.2 Fortio test profiles

| Profile                 | Fortio configuration                                           | Result                                   |
| ----------------------- | -------------------------------------------------------------- | ---------------------------------------- |
| Baseline HTTP RTT       | `-qps 10 -c 1`, at least 1,000 requests                        | Low-load request round-trip distribution |
| Fixed-load curve        | Shared QPS grid, `-c 50`, 60 s                                 | Achieved QPS, errors, p50/p90/p99/p99.9  |
| Connection scaling      | Fixed QPS at `-c 1,10,50,100`                                  | Effect of concurrency                    |
| Maximum HTTP throughput | `-qps 0` at fixed connections                                  | Maximum successful requests/s and MiB/s  |
| Raw TCP throughput      | `iperf3`, one and four streams, both directions, 60 s          | Gbit/s, retransmits, and host CPU cost   |
| Tail under constraints  | Fixed QPS while applying the constraint schedule in Section 11 | Tail amplification, errors, and recovery |

The initial shared QPS grid will be `100`, `500`, `1000`, `2000`, and `4000` QPS. A pilot will check whether this spans low load through saturation. The grid may be adjusted once and then frozen. Every runtime will receive the same offered loads; runtime-specific QPS values will not be selected after seeing final results.

Representative Fortio command:

```sh
fortio load \
  -qps "$QPS" \
  -c "$CONNECTIONS" \
  -t 60s \
  -json "$RESULT_FILE" \
  "http://$TARGET/fixed"
```

For p99.9 reporting, each measured trial must contain at least 100,000 completed requests. Otherwise p99.9 will be marked exploratory because too few observations exist in the upper 0.1%. Each profile will have 10 repetitions.

Fortio is selected because it can generate a specified QPS and stores latency histograms and percentiles in JSON. `wrk` is not selected because its closed-loop behavior can hide delayed request opportunities. `wrk2` is not rejected for this reason: it was specifically designed to compensate for coordinated omission. Fortio is preferred here for one consistent Go/JSON harness and fixed-QPS workflow.

`iperf3` will be used for the separate raw TCP bulk-throughput question, using JSON output, one and four parallel streams, and both send and reverse directions. A representative client command is:

```sh
iperf3 \
  --client "$TARGET_IP" \
  --time 60 \
  --parallel "$STREAMS" \
  --json
```

The server will run inside the system under test using the same resource profile. The exact `iperf3` version and binary will be identical where packaging permits; otherwise this test will remain in the extended matrix. `iperf3` does not provide HTTP request tail latency, so these results will be labeled network-layer throughput and will not replace Fortio results.

### 10.3 Network metrics

- Offered and achieved QPS
- Successful and failed requests
- HTTP request RTT mean, p50, p90, p95, p99, and p99.9
- Maximum latency and timeouts
- Response MiB/s
- Raw TCP Gbit/s and retransmits from `iperf3`
- TCP retransmits and socket errors
- Payload CPU and complete host CPU seconds
- CPU seconds per million successful requests
- Interface packet/byte counters
- Configuration path, including QEMU/KVM, Firecracker, TAP/virtio, and CNI details

<img width="1672" height="941" alt="image" src="https://github.com/user-attachments/assets/dedc2134-ecd2-4047-92b9-33bb6a719f6c" />


## 11. Tail-Latency and Stress Methodology

Tail behavior will be evaluated as a cross-cutting experiment rather than a new synthetic workload. The same CPU, storage, or HTTP workload will first run without contention, then under one declared constraint.

### 11.1 Constraint profiles

| Profile         | Constraint                                                 | Compared with                 |
| --------------- | ---------------------------------------------------------- | ----------------------------- |
| CPU-half        | Payload limited to 0.5 vCPU                                | Same workload at 1 vCPU       |
| Memory-half     | Payload limited to 512 MiB                                 | Same workload at 1 GiB        |
| Host-budget     | Entire sandbox placed under the shared host-budget profile | Payload-equivalent profile    |
| I/O-constrained | Fixed cgroup I/O limit or controlled competing I/O         | Unconstrained storage profile |
| Burst           | Launch `1`, `2`, `4`, and `8` instances simultaneously     | Serial launch                 |

### 11.2 Controlled spike test

For the HTTP workload, Fortio will apply a fixed offered load for 90 seconds. The sequence will be:

1. `0-30 s`: normal resource profile
2. `30-45 s`: apply the declared CPU, memory, or I/O constraint
3. `45-90 s`: restore the original profile and observe recovery

Only one constraint changes in a trial. The harness will record the exact monotonic timestamp of the change. Results will be divided into before, during, and recovery windows and will report p99/p99.9, achieved QPS, errors, and time to return within 10% of the pre-spike p99.

OOM tests will not be combined with service-latency tests. They will run separately with strict top-level host limits, timeouts, and cleanup checks.

## 12. Variance Handling

### 12.1 Repetitions and warm-up

| Experiment             |                                 Warm-up |                                    Measured repetitions |
| ---------------------- | --------------------------------------: | ------------------------------------------------------: |
| Lifecycle warm         |                        3 untimed trials |                                                      30 |
| Lifecycle runtime-cold |                                    None |                              10 independent cold blocks |
| CPU modes              |                          5 s in-process |                                       10 trials of 30 s |
| Memory regular modes   |                    One untimed mode run |                                                      10 |
| Storage                |                    10 s `fio` ramp time |                                       10 trials of 60 s |
| Network                | Connection/setup warm-up outside result | 10 trials, at least 60 s and 100,000 requests for p99.9 |
| Density and OOM        |                     One small smoke run |                                    At least 5 per level |

These are starting counts. The pilot will calculate the coefficient of variation and bootstrap confidence-interval width. If an important metric remains unstable, I will increase repetitions or duration for every runtime in that experiment, not only for the runtime with unfavorable results.

### 12.2 Randomized balanced blocks

Trials will not run as all `runc`, then all Kata, and so on. Each block will contain one trial for every runtime/configuration in randomized order. This reduces bias from temperature, page-cache evolution, background activity, and time of day. The random seed and actual order will be stored.

### 12.3 Invalid trials and outliers

A trial may be invalidated only for a documented external reason such as collector failure, corrupted output, wrong image digest, unrelated host activity above the preflight threshold, or load-generator saturation. The raw trial and exclusion reason will remain in the dataset.

Runtime timeouts, crashes, OOM kills, startup failures, and cleanup failures are results. They will not be removed as statistical outliers.

## 13. Reporting

### 13.1 Absolute values and ratios

Every result table will show absolute values, trial count, failures, and a normalized comparison.

For metrics where higher is better:

$$
\text{Performance ratio} = \frac{\text{runtime result}}{\text{baseline result}}
$$

For latency or resource cost where lower is better:

$$
\text{Cost ratio} = \frac{\text{runtime cost}}{\text{baseline cost}}
$$

For example, a CPU throughput ratio of `0.80x` means the runtime delivers 80% of native throughput, while a lifecycle cost ratio of `1.40x` means the operation takes 40% longer than `runc`.

Native will be the baseline for CPU, memory-performance, storage, and network workloads. `runc` will also be shown as the OCI baseline. Lifecycle ratios will use `runc` because native Linux has no OCI lifecycle.

### 13.2 Statistical summary

For each experiment I will report:

- Attempted, successful, failed, and timed-out trials
- Median and arithmetic mean
- Standard deviation and coefficient of variation
- p95 and p99 across independent trial summaries where meaningful
- Per-operation p50/p90/p95/p99/p99.9 for Fortio and `fio`
- 95% bootstrap confidence intervals
- Absolute result, native ratio, and `runc` ratio

Tail percentiles from millions of operations inside one run will not be treated as millions of independent runtime trials. Per-request distributions and across-trial uncertainty will be presented separately.

### 13.3 Planned figures

- Lifecycle ECDF and median/p95 comparison; p99 only when the sample count is increased enough to support it
- CPU threads versus throughput and scaling efficiency
- Useful work versus total host CPU cost
- Touched memory versus total host memory increase
- Instance count versus host memory and readiness p99
- Storage IOPS/throughput with latency percentile curves
- Offered QPS versus achieved QPS, p99, and errors
- Before/during/recovery tail-latency time series

## 14. Trial Procedure

Each measured trial will follow the same sequence:

1. Validate environment, versions, image digest, available resources, and absence of stale instances.]
2. Create a unique trial ID and write the frozen manifest.
3. Perform the declared warm-up without saving it as a measured sample.
4. Start collectors before runtime create.
5. Run the workload with a fixed timeout.
6. Save unmodified stdout, stderr, tool JSON, runtime logs, and collector JSONL.
7. Validate operation count, checksum, duration, and runtime identity.
8. Stop and delete the workload.
9. Record success or the exact failed stage.
10. Wait the fixed cool-down interval before the next randomized trial.


## References

1. Open Container Initiative, [Runtime and lifecycle specification](https://github.com/opencontainers/runtime-spec/blob/main/runtime.md).
2. containerd, [Getting started: containers and tasks](https://github.com/containerd/containerd/blob/main/docs/getting-started.md).
3. containerd, [Runtime v2 task flow and events](https://github.com/containerd/containerd/blob/main/docs/runtime-v2.md).
4. Linux Kernel, [Control Group v2 documentation](https://docs.kernel.org/admin-guide/cgroup-v2.html).
5. Linux Kernel, [Pressure Stall Information](https://www.kernel.org/doc/html/latest/accounting/psi.html).
6. Linux Kernel, [`/proc` and `smaps_rollup` documentation](https://docs.kernel.org/filesystems/proc.html).
7. Fortio, [Fortio documentation](https://fortio.org/).
8. `wrk2`, [Constant-throughput and coordinated-omission methodology](https://github.com/giltene/wrk2).
9. fio, [fio documentation](https://fio.readthedocs.io/en/latest/fio_doc.html).
10. sysbench, [sysbench documentation](https://github.com/akopytov/sysbench).
11. Containerd, [Runtime v2](https://github.com/containerd/containerd/blob/main/docs/runtime-v2.md).
