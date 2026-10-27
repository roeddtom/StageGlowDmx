param(
    [ValidateSet("host", "pi2")]
    [string]$Target = "host",
    [switch]$SkipInstall
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    Push-Location (Join-Path $root "web")
    try {
        if (-not $SkipInstall) {
            & npm.cmd ci
            if ($LASTEXITCODE -ne 0) { throw "Frontend dependency installation failed." }
        }
        & npm.cmd run build
        if ($LASTEXITCODE -ne 0) { throw "Frontend build failed." }
    } finally {
        Pop-Location
    }

    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw "Backend tests failed." }

    $previousOS = $env:GOOS
    $previousArch = $env:GOARCH
    $previousArm = $env:GOARM
    $previousCGO = $env:CGO_ENABLED
    try {
        $env:CGO_ENABLED = "0"
        $binary = "stageglow"
        if ($Target -eq "pi2") {
            $env:GOOS = "linux"
            $env:GOARCH = "arm"
            $env:GOARM = "7"
            $binary = "stageglow-linux-armv7"
        } elseif ($env:OS -eq "Windows_NT") {
            $binary = "stageglow.exe"
        }
        New-Item -ItemType Directory -Force -Path (Join-Path $root "build") | Out-Null
        & go build -trimpath -o (Join-Path $root "build\$binary") ./cmd/stageglow
        if ($LASTEXITCODE -ne 0) { throw "Backend build failed." }
    } finally {
        $env:GOOS = $previousOS
        $env:GOARCH = $previousArch
        $env:GOARM = $previousArm
        $env:CGO_ENABLED = $previousCGO
    }
} finally {
    Pop-Location
}
