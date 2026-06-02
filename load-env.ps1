Get-Content .env | Where-Object { $_ -match '^\s*[^#].*=.*$' } | ForEach-Object {
    $name, $value = $_ -split '=', 2
    Set-Item -Path "Env:$($name.Trim())" -Value $value.Trim()
}