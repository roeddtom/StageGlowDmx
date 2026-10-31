param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[a-zA-Z0-9][a-zA-Z0-9.-]*$')]
    [string]$PiAddress,
    [ValidatePattern('^[a-z_][a-z0-9_-]*$')]
    [string]$User = "stageglow",
    [string]$IdentityFile = (Join-Path $env:USERPROFILE ".ssh\stageglow_pi"),
    [string]$Bundle = (Join-Path (Split-Path -Parent $PSScriptRoot) "build\stageglow-pi2.tar.gz"),
    [switch]$UploadOnly
)

$ErrorActionPreference = "Stop"
foreach ($tool in @("ssh.exe", "scp.exe")) {
    if (-not (Get-Command $tool -CommandType Application -ErrorAction SilentlyContinue)) {
        throw "$tool was not found. Install the Windows OpenSSH Client."
    }
}
if (-not (Test-Path -LiteralPath $Bundle -PathType Leaf)) {
    throw "Pi bundle not found: $Bundle. Run .\scripts\build.ps1 -Target pi2 first."
}
if (-not (Test-Path -LiteralPath $IdentityFile -PathType Leaf)) {
    throw "SSH key not found: $IdentityFile. Pass -IdentityFile with your SSH key path."
}
$Bundle = (Resolve-Path -LiteralPath $Bundle).Path
$IdentityFile = (Resolve-Path -LiteralPath $IdentityFile).Path
$connection = "$User@$PiAddress"
$sshOptions = @("-i", $IdentityFile, "-o", "ConnectTimeout=10")
$directory = "stageglow-update-" + [Guid]::NewGuid().ToString("N")

if (-not $UploadOnly) {
    Write-Host "This restarts StageGlow. Black out and disable USB, and export a backup first."
    if ((Read-Host "Type UPDATE to proceed") -cne "UPDATE") {
        Write-Host "Update cancelled."
        return
    }
}

& ssh.exe @sshOptions $connection "umask 077; mkdir ~/$directory"
if ($LASTEXITCODE -ne 0) { throw "Could not prepare the Pi upload directory." }
& scp.exe @sshOptions $Bundle "${connection}:$directory/bundle.tar.gz"
if ($LASTEXITCODE -ne 0) { throw "Pi bundle upload failed." }
& ssh.exe @sshOptions $connection "set -eu; cd ~/$directory; tar -xzf bundle.tar.gz; sha256sum -c SHA256SUMS; sh -n install.sh"
if ($LASTEXITCODE -ne 0) { throw "Pi bundle verification failed; nothing installed." }

if ($UploadOnly) {
    Write-Host "Verified upload. Install later with:"
    Write-Host "ssh -t -i `"$IdentityFile`" $connection `"cd ~/$directory; sudo sh ./install.sh`""
    return
}

& ssh.exe @sshOptions -t $connection "cd ~/$directory; sudo sh ./install.sh"
if ($LASTEXITCODE -ne 0) {
    throw "Installation failed. Inspect the terminal output and 'journalctl -u stageglow -n 100 --no-pager' on the Pi. Uploaded files remain in ~/$directory."
}
Write-Host "Updated. Settings and HTTPS overrides retained; USB starts disabled."
Write-Host "HTTP: http://$PiAddress (if enabled); HTTPS: https://${PiAddress}:8443 (if configured)."
Write-Host "Refresh/reopen the browser app to load the new version."
