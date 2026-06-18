$ErrorActionPreference = 'Stop'

$env:HTTP_ADDR = ":8080"
$env:MMAP_FILE = "data/big.bin"
$env:MMAP_SIZE_MB = "500"
$env:SAMPLES_DEFAULT = "1000"

go run ./cmd/coldstart


