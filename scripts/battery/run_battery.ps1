# Executa a bateria Python a partir de backend-engine-go (os .py ficam na raiz do repo).
# Uso (PowerShell): .\scripts\battery\run_battery.ps1
#
# Python pode vir de:
#   - variavel de ambiente PYTHON_EXE (caminho completo para python.exe)
#   - PATH: python, py -3, python3
#   - instalacao tipica em %LocalAppData%\Programs\Python\ (mesmo sem PATH nesta sessao)

$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
$repoRoot = (Resolve-Path (Join-Path $here "..\..\..")).Path
$batteryDir = Join-Path $repoRoot "scripts\battery"
$req = Join-Path $batteryDir "requirements.txt"
$mass = Join-Path $batteryDir "mass_test.py"

if (-not (Test-Path $mass)) {
    Write-Error "Nao encontrado: $mass (repo root esperado: $repoRoot)"
    exit 1
}

function Test-PythonExe {
    param([string] $ExePath)
    if (-not $ExePath -or -not (Test-Path -LiteralPath $ExePath)) { return $false }
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    try {
        & $ExePath -c "import sys" 2>&1 | Out-Null
        if ($LASTEXITCODE -eq 0) { return $true }
        # PowerShell 5.1 as vezes nao define LASTEXITCODE; $? ajuda
        return [bool]$?
    } catch {
        return $false
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Test-PyLauncher {
    if (-not (Get-Command "py" -ErrorAction SilentlyContinue)) { return $false }
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    try {
        & py -3 -c "import sys" 2>&1 | Out-Null
        if ($LASTEXITCODE -eq 0) { return $true }
        return [bool]$?
    } catch {
        return $false
    } finally {
        $ErrorActionPreference = $prev
    }
}

function Find-PythonExePath {
    $envPath = $env:PYTHON_EXE
    if ($envPath -and (Test-Path -LiteralPath $envPath) -and (Test-PythonExe $envPath)) {
        return $envPath
    }

    foreach ($name in @("python", "python3")) {
        $cmd = Get-Command $name -ErrorAction SilentlyContinue
        if (-not $cmd) { continue }
        $p = $cmd.Source
        if ($p -and (Test-PythonExe $p)) { return $p }
    }

    if (Test-PyLauncher) {
        $prev = $ErrorActionPreference
        $ErrorActionPreference = "SilentlyContinue"
        try {
            $pyPath = & py -3 -c "import sys; print(sys.executable)" 2>&1
            if ($LASTEXITCODE -eq 0 -and $pyPath -and (Test-Path -LiteralPath $pyPath.Trim())) {
                return $pyPath.Trim()
            }
        } finally {
            $ErrorActionPreference = $prev
        }
        return "__PY_LAUNCHER__"
    }

    $patterns = @(
        (Join-Path $env:LocalAppData "Programs\Python\Python*\python.exe"),
        (Join-Path $env:ProgramFiles "Python*\python.exe"),
        (Join-Path ${env:ProgramFiles(x86)} "Python*\python.exe")
    )
    foreach ($pat in $patterns) {
        $hits = @(Get-Item $pat -ErrorAction SilentlyContinue | Sort-Object FullName -Descending)
        foreach ($c in $hits) {
            if ($c.FullName -match "WindowsApps") { continue }
            if (Test-PythonExe $c.FullName) { return $c.FullName }
        }
    }

    return $null
}

$pythonExe = Find-PythonExePath
if (-not $pythonExe) {
    Write-Error @"
Python 3 nao encontrado.

Opcoes:
  1) Instale de https://www.python.org/downloads/ e marque ""Add python.exe to PATH"".
  2) Feche e reabra o PowerShell depois de instalar.
  3) Defina o caminho completo antes de correr o script, por exemplo:
       `$env:PYTHON_EXE = ""$env:LocalAppData\Programs\Python\Python312\python.exe""
       .\scripts\battery\run_battery.ps1
"@
    exit 1
}

Write-Host "Usando Python: $pythonExe"
Write-Host "Battery dir: $batteryDir"

if ($pythonExe -eq "__PY_LAUNCHER__") {
    & py -3 -m pip install -r $req
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Push-Location $batteryDir
    try {
        & py -3 $mass
        exit $LASTEXITCODE
    } finally { Pop-Location }
}

& $pythonExe -m pip install -r $req
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Push-Location $batteryDir
try {
    & $pythonExe $mass
    exit $LASTEXITCODE
} finally { Pop-Location }
