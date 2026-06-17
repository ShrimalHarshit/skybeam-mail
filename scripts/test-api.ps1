# scripts\test-api.ps1 - SkyBeam API smoke test
# Run from d:\SkyMail after the API is up on :8080
# Usage: .\scripts\test-api.ps1

param(
    [string]$Email    = "alice@skybeam.live",
    [string]$Password = "testpass123",
    [string]$BaseUrl  = "http://localhost:8080"
)

$API = "$BaseUrl/api/v1"
$pass = $true

function Test-Step($label, $block) {
    Write-Host "`n-- $label " -ForegroundColor Cyan -NoNewline
    try {
        $result = & $block
        Write-Host "OK" -ForegroundColor Green
        return $result
    } catch {
        Write-Host "FAIL  $_" -ForegroundColor Red
        $script:pass = $false
        return $null
    }
}

function Assert($condition, $msg) {
    if (-not $condition) { throw $msg }
}

# --- 1. Health -------------------------------------------------------
$health = Test-Step "GET /health" {
    $r = Invoke-RestMethod "$BaseUrl/health"
    Assert ($r.status -eq "ok") "Expected status=ok, got: $($r.status)"
    $r
}

# --- 2. Login --------------------------------------------------------
$token = Test-Step "POST /auth/login" {
    $body = @{ email = $Email; password = $Password } | ConvertTo-Json
    $r = Invoke-RestMethod "$API/auth/login" -Method POST `
         -ContentType "application/json" -Body $body
    Assert ($r.token -ne $null) "No token in response"
    Assert ($r.account.email -eq $Email) "Email mismatch"
    $r.token
}

if (-not $token) {
    Write-Host "`n[FATAL] Cannot continue without a token." -ForegroundColor Red
    exit 1
}

$headers = @{ Authorization = "Bearer $token" }

# --- 3. Me -----------------------------------------------------------
Test-Step "GET /auth/me" {
    $r = Invoke-RestMethod "$API/auth/me" -Headers $headers
    Assert ($r.email -eq $Email) "Email mismatch in /me"
    $r
} | Out-Null

# --- 4. Folders ------------------------------------------------------
$folders = Test-Step "GET /folders" {
    $r = Invoke-RestMethod "$API/folders" -Headers $headers
    Assert ($r.data -ne $null) "No data array in folder response"
    $r.data
}

if ($folders) {
    $inbox = $folders | Where-Object { $_.name -eq "INBOX" }
    Assert ($inbox -ne $null) "INBOX folder not found"
    Write-Host "   Folders: $($folders.Count) found, INBOX unread=$($inbox.unread)"
}

# --- 5. Messages -----------------------------------------------------
$messages = Test-Step "GET /messages?folder=INBOX" {
    $r = Invoke-RestMethod "$API/messages?folder=INBOX" -Headers $headers
    # PowerShell deserialises an empty JSON array [] as $null — check the key
    # exists on the object instead of checking the value isn't null.
    Assert ($r.PSObject.Properties.Name -contains "data") "No 'data' key in message response"
    # Treat null (empty array) as an empty collection — 0 messages is valid.
    if ($r.data -eq $null) { return @() }
    $r.data
}

Write-Host "   Messages in INBOX: $($messages.Count)"

# --- 6. Search -------------------------------------------------------
Test-Step "GET /search?q=test" {
    $r = Invoke-RestMethod "$API/search?q=test" -Headers $headers
    Assert ($r.query -eq "test") "Query echo missing"
} | Out-Null

# --- 7. Events -------------------------------------------------------
$events = Test-Step "GET /events" {
    $r = Invoke-RestMethod "$API/events?limit=5" -Headers $headers
    Assert ($r.data -ne $null) "No data array in events response"
    $r.data
}
Write-Host "   Events recorded: $($events.Count)"

# --- 8. Create folder ------------------------------------------------
$testFolder = "TestFolder_$(Get-Random -Maximum 9999)"
Test-Step "POST /folders (create '$testFolder')" {
    $body = @{ name = $testFolder } | ConvertTo-Json
    $r = Invoke-RestMethod "$API/folders" -Method POST `
         -ContentType "application/json" -Body $body -Headers $headers
    Assert ($r.name -eq $testFolder) "Folder name mismatch"
} | Out-Null

# --- 9. Delete that folder -------------------------------------------
Test-Step "DELETE /folders/$testFolder" {
    Invoke-RestMethod "$API/folders/$testFolder" -Method DELETE -Headers $headers
} | Out-Null

# --- 10. Logout ------------------------------------------------------
Test-Step "DELETE /auth/logout" {
    Invoke-RestMethod "$API/auth/logout" -Method DELETE -Headers $headers
} | Out-Null

# --- 11. Confirm token is invalid after logout -----------------------
Test-Step "Verify token invalidated" {
    try {
        Invoke-RestMethod "$API/auth/me" -Headers $headers -ErrorAction Stop
        throw "Expected 401, got 200"
    } catch {
        if ($_.Exception.Response.StatusCode.value__ -eq 401) { return }
        throw "Expected 401, got: $_"
    }
} | Out-Null

# --- Summary ---------------------------------------------------------
Write-Host ""
if ($pass) {
    Write-Host "==============================================" -ForegroundColor Green
    Write-Host "  All API smoke tests PASSED" -ForegroundColor Green
    Write-Host "==============================================" -ForegroundColor Green
} else {
    Write-Host "==============================================" -ForegroundColor Red
    Write-Host "  Some tests FAILED -- check output above" -ForegroundColor Red
    Write-Host "==============================================" -ForegroundColor Red
    exit 1
}
