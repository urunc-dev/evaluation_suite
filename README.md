# OCI runtime evaluation harness

This repository contains a reproducible evaluation harness for comparing OCI
container runtimes. It currently supports experiments using:

- [urunc](https://github.com/urunc-dev/urunc)
- [runc](https://github.com/opencontainers/runc)
- [Kata Containers](https://katacontainers.io/)
- [gVisor/runsc](https://gvisor.dev/)

The harness runs lifecycle and storage benchmarks through containerd, records
the host environment, and writes machine-readable results for the included
analysis notebooks.

## Requirements

Run the harness on a Linux host with:

- Go 1.26.4 or a compatible newer Go release (see `go.mod`)
- containerd running with the runtime handlers you intend to benchmark
- cgroup v2 mounted
- `ctr` available in `PATH`
- the required runtime binaries installed (`runc`, `containerd-shim-kata-v2`,
  `runsc`, and/or the urunc components)
- benchmark images accessible to containerd

The gVisor-specific execution path also uses `sudo`, `skopeo`, `umoci`, and
`nerdctl`. Storage images must contain `fio`; a host installation of `fio` is
useful for readiness checks but is not required when it is present in the
image.

The harness normally needs root privileges to access containerd, configure
runtime resources, and collect the full set of environment data.

## Build

From the repository root, build the harness binary:

```bash
go build -o harness cmd/harness/main.go
```

To confirm that it was built successfully:

```bash
./harness --help
```

## Quick start

The repository includes an example manifest at `experiment.yml`.

First validate it and inspect the concrete trials that it will create:

```bash
./harness validate -f experiment.yml
./harness plan -f experiment.yml -o summary
```

Check whether the machine is ready. Running this check with the same privileges
as the benchmark gives the most representative result:

```bash
sudo ./harness doctor -f experiment.yml
```

Then run the evaluation:

```bash
sudo ./harness run -f experiment.yml
```

By default, artifacts are written beneath `./results`. To use another location,
pass `--results-dir`, for example:

```bash
sudo ./harness run -f experiment.yml --results-dir /path/to/results
```

Both `plan` and `run` support JSON output with `-o json`. This changes terminal
output only; `run` still writes its artifact files.

## Results

Each run creates a UTC timestamped directory like:

```text
results/
└── run-20260721T120000Z/
    ├── environment.json
    ├── manifest.yaml
    ├── plan.json
    └── run.json
```

- `run.json` contains the evaluation results, including every trial, its
  status, duration, and recorded runtime stages.
- `environment.json` contains the captured host environment, such as OS,
  kernel, CPU, memory, NUMA, swap, Go, and tool information. Capture warnings
  are recorded there as well.
- `manifest.yaml` is the frozen copy of the manifest used for the run.
- `plan.json` is the fully expanded list of trials generated from the manifest.

The summary printed by `harness run` includes the exact artifact directory.

## Writing `experiment.yml`

An experiment manifest has two top-level sections: `runtimes` and
`experiments`.

### Complete example

```yaml
runtimes:
  - name: runc
    handler: io.containerd.runc.v2

  - name: kata
    handler: io.containerd.kata.v2

  - name: runsc
    handler: io.containerd.runsc.v1

  - name: urunc
    handler: io.containerd.urunc.v2

experiments:
  lifecycle:
    workloads:
      default:
        image: docker.io/library/nginx:alpine

      # urunc needs a runtime-specific image, so this workload replaces the
      # default lifecycle workload for urunc only.
      other:
        - image: docker.io/library/minimalc:test
          runtime: urunc

  storage:
    workloads:
      default:
        image: docker.io/jimjuniorb/fio:0.1
        snapshotter: overlayfs

      other:
        - image: docker.io/jimjuniorb/fio-urunc:0.4
          runtime: urunc
          snapshotter: devmapper
```

Only `lifecycle` and `storage` currently have benchmark adapters, so use those
names as the experiment keys.

### 1. Declare runtimes

Every runtime entry requires:

- `name`: the short name referenced by workloads and shown in results. Names
  must be unique.
- `handler`: the corresponding containerd runtime handler configured on the
  host.

Declare only the runtimes that are installed and configured on the benchmark
machine.

### 2. Add an experiment and its default workload

Every experiment requires `workloads.default.image`. The harness expands the
default workload into one trial for each declared runtime:

```yaml
experiments:
  lifecycle:
    workloads:
      default:
        image: docker.io/library/nginx:alpine
```

With four declared runtimes, this produces four lifecycle trials using the
same image.

For a storage experiment, set a snapshotter and use an image containing `fio`:

```yaml
experiments:
  storage:
    workloads:
      default:
        image: docker.io/example/fio:latest
        snapshotter: overlayfs
```

### 3. Add runtime-specific or additional workloads

Use `workloads.other` when a runtime needs a different image or snapshotter:

```yaml
other:
  - image: docker.io/example/fio-urunc:latest
    runtime: urunc
    snapshotter: devmapper
```

When an `other` workload specifies `runtime`, it creates one trial for that
runtime and suppresses that runtime's default trial. The runtime name must match
an entry in `runtimes` exactly.

When `runtime` is omitted, the additional workload runs once for every declared
runtime; it does not replace the default workload:

```yaml
other:
  - image: docker.io/example/second-workload:latest
```

### Validate and preview the manifest

Always validate and preview a new manifest before running it:

```bash
./harness validate -f experiment.yml
./harness plan -f experiment.yml -o summary
sudo ./harness doctor -f experiment.yml
```

`validate` catches missing fields, duplicate or unknown runtime names, invalid
ports, and incomplete volume declarations. `plan` shows exactly which runtime,
image, workload, and handler each generated trial will use. `doctor` checks the
host, containerd configuration, result directory, ports, volumes, and relevant
tools.

## Analyze results with the notebooks

The notebooks expect a file named `run.json` in the notebook's working
directory. Copy the output from the run you want to analyze into `notebooks/`:

```bash
cp results/run-<timestamp>/run.json notebooks/run.json
```

Choose the notebook that matches the experiment:

- [`notebooks/oci_lifecycle_latency_analysis.ipynb`](notebooks/oci_lifecycle_latency_analysis.ipynb)
  analyzes lifecycle latency results.
- [`notebooks/fio_runtime_analysis.ipynb`](notebooks/fio_runtime_analysis.ipynb)
  analyzes storage/FIO results.

### Recommended: Google Colab

Colab is the easiest option because it does not require a local Python or
Jupyter installation:

- [Open the lifecycle analysis notebook in Colab](https://colab.research.google.com/github/jim-junior/evaluation_suite/blob/main/notebooks/oci_lifecycle_latency_analysis.ipynb)
- [Open the storage/FIO analysis notebook in Colab](https://colab.research.google.com/github/jim-junior/evaluation_suite/blob/main/notebooks/fio_runtime_analysis.ipynb)

After Colab opens, use the **Files** panel to upload your `run.json` into the
session's working directory, then select **Runtime > Run all**. Colab storage is
temporary, so download any generated figures or tables you want to keep before
ending the session.

### Run locally with uv

The project includes `pyproject.toml` and `uv.lock` for a reproducible notebook
environment. Install [uv](https://docs.astral.sh/uv/), then run:

```bash
uv python install 3.14
uv sync --locked
cp results/run-<timestamp>/run.json notebooks/run.json
cd notebooks
uv run --project .. jupyter lab
```

Open the appropriate notebook in the Jupyter interface and run all cells. Stop
the server with `Ctrl+C` when finished.
