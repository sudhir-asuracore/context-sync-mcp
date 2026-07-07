$repo = "sudhir-asuracore/context-sync-mcp"
$binary = "contextsync"
$filename = "${binary}-windows-amd64.exe"
$url = "https://github.com/$repo/releases/latest/download/$filename"

$installDir = Join-Path $HOME "bin"
if (!(Test-Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}

$destPath = Join-Path $installDir "$binary.exe"

Write-Host "Installing ${binary} to ${destPath}..."
Write-Host "Downloading from: ${url}"

try {
    Invoke-WebRequest -Uri $url -OutFile $destPath -ErrorAction Stop
} catch {
    Write-Error "Failed to download ${binary}. Please check your internet connection or the repository URL."
    exit 1
}

# Add to PATH if not already there
$path = [Environment]::GetEnvironmentVariable("Path", "User")
if ($path -split ';' -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", "$path;$installDir", "User")
    Write-Host "Added ${installDir} to User PATH."
    Write-Host "NOTE: You may need to restart your terminal for the changes to take effect."
}

Write-Host "Installation complete! You can now run '${binary}'."
