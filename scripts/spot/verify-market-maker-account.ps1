[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [uri]$BaseUrl,

    [Parameter(Mandatory = $true)]
    [ValidateRange(1, [long]::MaxValue)]
    [long]$UserId,

    [Parameter(Mandatory = $true)]
    [string]$ApiKeyFile,

    [decimal]$Target = 1000000,
    [switch]$TradingSmoke,
    [int]$MarketId = 1,
    [ValidateSet('BUY', 'SELL')]
    [string]$SmokeSide = 'BUY',
    [string]$SmokePrice = '1.00000000',
    [string]$SmokeQuantity = '0.01000000'
)

$ErrorActionPreference = 'Stop'

if ($BaseUrl.Scheme -ne 'https' -and -not (
    $BaseUrl.Scheme -eq 'http' -and $BaseUrl.Host -in @('127.0.0.1', 'localhost', '::1')
)) {
    throw 'BaseUrl must use HTTPS or loopback HTTP.'
}
if (-not (Test-Path -LiteralPath $ApiKeyFile -PathType Leaf)) {
    throw 'ApiKeyFile does not exist.'
}
$apiKey = (Get-Content -LiteralPath $ApiKeyFile -Raw).Trim()
if ([string]::IsNullOrWhiteSpace($apiKey)) {
    throw 'ApiKeyFile is empty.'
}

$base = $BaseUrl.AbsoluteUri.TrimEnd('/')
$client = [Net.Http.HttpClient]::new()
$client.DefaultRequestHeaders.Add('X-API-Key', $apiKey)
$client.Timeout = [TimeSpan]::FromSeconds(15)
$script:passed = 0

function Invoke-Api {
    param(
        [Parameter(Mandatory = $true)][Net.Http.HttpMethod]$Method,
        [Parameter(Mandatory = $true)][string]$Path,
        [object]$Body
    )
    $request = [Net.Http.HttpRequestMessage]::new($Method, "$base$Path")
    try {
        if ($null -ne $Body) {
            $json = $Body | ConvertTo-Json -Compress -Depth 10
            $request.Content = [Net.Http.StringContent]::new(
                $json, [Text.Encoding]::UTF8, 'application/json')
        }
        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        try {
            $text = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            $parsed = $null
            if (-not [string]::IsNullOrWhiteSpace($text)) {
                try { $parsed = $text | ConvertFrom-Json -Depth 20 } catch { $parsed = $null }
            }
            return [pscustomobject]@{
                Status = [int]$response.StatusCode
                Body = $text
                Json = $parsed
            }
        } finally {
            $response.Dispose()
        }
    } finally {
        $request.Dispose()
    }
}

function Assert-Status {
    param([object]$Response, [int]$Expected, [string]$Name)
    if ($Response.Status -ne $Expected) {
        throw "$Name expected HTTP $Expected but received HTTP $($Response.Status)."
    }
    $script:passed++
    Write-Output "PASS  $Name (HTTP $Expected)"
}

function Get-BalanceSnapshot {
    param([object]$Account)
    $items = foreach ($asset in @($Account.assets)) {
        [pscustomobject]@{
            assetId = [int]$asset.assetId
            available = [decimal]$asset.available
            locked = [decimal]$asset.locked
            total = [decimal]$asset.total
        }
    }
    return @($items | Sort-Object assetId)
}

function Assert-TargetBalances {
    param([object]$Account)
    $snapshot = @(Get-BalanceSnapshot $Account)
    if ($snapshot.Count -ne 4 -or (@($snapshot.assetId) -join ',') -ne '0,1,2,3') {
        throw 'Balance response must contain exactly assetIds 0,1,2,3.'
    }
    foreach ($asset in $snapshot) {
        if ($asset.available -ne $Target -or $asset.locked -ne 0 -or $asset.total -ne $Target) {
            throw "assetId $($asset.assetId) does not have the exact target balance."
        }
    }
    $script:passed++
    Write-Output "PASS  assets 0-3 each available=$Target locked=0 total=$Target"
}

try {
    $identity = Invoke-Api ([Net.Http.HttpMethod]::Get) '/api/v1/auth/me'
    Assert-Status $identity 200 'API-key identity'
    if ([long]$identity.Json.userId -ne $UserId) {
        throw "Authenticated userId does not match requested account."
    }

    $self = Invoke-Api ([Net.Http.HttpMethod]::Get) "/api/v1/accounts/$UserId"
    Assert-Status $self 200 'self account read'
    Assert-TargetBalances $self.Json

    $balances = Invoke-Api ([Net.Http.HttpMethod]::Get) '/api/v1/balances'
    Assert-Status $balances 200 'self balance shortcut'
    Assert-TargetBalances $balances.Json

    $otherUserId = if ($UserId -eq [long]::MaxValue) { $UserId - 1 } else { $UserId + 1 }
    Assert-Status (Invoke-Api ([Net.Http.HttpMethod]::Get) "/api/v1/accounts/$otherUserId") `
        403 'cross-account read denied'

    $movement = @{ assetId = 0; amount = '0.00000001' }
    Assert-Status (Invoke-Api ([Net.Http.HttpMethod]::Post) "/api/v1/accounts/$UserId/deposit" $movement) `
        403 'internal deposit denied'
    Assert-Status (Invoke-Api ([Net.Http.HttpMethod]::Post) "/api/v1/accounts/$UserId/withdraw" $movement) `
        403 'internal withdrawal denied'

    $withdrawalBase = @{
        clientWithdrawalId = "permission-check-$([guid]::NewGuid().ToString('N'))"
        destinationAddress = '0x0000000000000000000000000000000000000001'
        assetId = 0
        amount = '0.00000001'
    }
    $directWithdrawal = $withdrawalBase.Clone()
    $directWithdrawal.targetChainId = 7260
    Assert-Status (Invoke-Api ([Net.Http.HttpMethod]::Post) '/api/v1/neco/withdrawals' `
        $directWithdrawal) 403 'NECO direct withdrawal denied'
    $bridgeWithdrawal = $withdrawalBase.Clone()
    $bridgeWithdrawal.targetChainId = 56
    $bridgeWithdrawal.clientWithdrawalId = "permission-check-$([guid]::NewGuid().ToString('N'))"
    Assert-Status (Invoke-Api ([Net.Http.HttpMethod]::Post) '/api/v1/neco/withdrawals' `
        $bridgeWithdrawal) 403 'bridge withdrawal denied'
    Assert-Status (Invoke-Api ([Net.Http.HttpMethod]::Get) '/api/v1/admin/risk/config') `
        403 'admin access denied'

    if ($TradingSmoke) {
        $before = (Get-BalanceSnapshot $self.Json) | ConvertTo-Json -Compress
        $clientOrderId = "mm-smoke-$([guid]::NewGuid().ToString('N'))"
        $orderBody = @{
            userId = $UserId
            marketId = $MarketId
            side = $SmokeSide
            orderType = 'LIMIT'
            timeInForce = 'GTC'
            price = $SmokePrice
            quantity = $SmokeQuantity
            clientOrderId = $clientOrderId
        }
        $created = Invoke-Api ([Net.Http.HttpMethod]::Post) '/api/v1/orders' $orderBody
        if ($created.Status -notin @(200, 201) -or -not $created.Json.accepted) {
            throw "Trading smoke order was rejected (HTTP $($created.Status))."
        }
        $orderId = [long]$created.Json.omsOrderId
        $script:passed++
        Write-Output "PASS  non-crossing limit order accepted (omsOrderId=$orderId)"

        $cancelled = Invoke-Api ([Net.Http.HttpMethod]::Delete) "/api/v1/orders/$orderId"
        Assert-Status $cancelled 200 'smoke order cancellation'

        $deadline = [DateTime]::UtcNow.AddSeconds(15)
        do {
            Start-Sleep -Milliseconds 250
            $order = Invoke-Api ([Net.Http.HttpMethod]::Get) "/api/v1/orders/$orderId"
            $afterResponse = Invoke-Api ([Net.Http.HttpMethod]::Get) "/api/v1/accounts/$UserId"
            $after = (Get-BalanceSnapshot $afterResponse.Json) | ConvertTo-Json -Compress
            $cancelSettled = $order.Status -eq 200 -and $order.Json.status -eq 'CANCELLED' `
                -and [decimal]$order.Json.filledQty -eq 0 -and $after -eq $before
        } until ($cancelSettled -or [DateTime]::UtcNow -ge $deadline)
        if (-not $cancelSettled) {
            throw 'Smoke order did not settle to CANCELLED with zero fill and unchanged balances.'
        }
        $script:passed++
        Write-Output 'PASS  smoke order had no fill and balances returned to their original values'
    }

    Write-Output "PASS: market-maker account verification completed ($script:passed checks)."
} finally {
    $apiKey = $null
    $client.Dispose()
}
