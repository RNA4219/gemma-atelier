param(
    [string]$JdkRoot = $env:JAVA_HOME,
    [string]$AndroidSdk = "$env:LOCALAPPDATA/Android/Sdk",
    [string]$KeyStore = ''
)
$ErrorActionPreference='Stop'
if (-not $JdkRoot -or -not (Test-Path -LiteralPath "$JdkRoot/bin/javac.exe")) { throw 'JDK17のパスを-JdkRootまたはJAVA_HOMEで指定してください。' }
$taskRoot=Split-Path -Parent $PSScriptRoot
$sdk=$AndroidSdk
$bt=Join-Path $sdk 'build-tools/35.0.0'
$android=Join-Path $sdk 'platforms/android-35/android.jar'
$jdk=$JdkRoot
$env:JAVA_HOME=$jdk
$env:PATH="$jdk/bin;"+$env:PATH
$project=Join-Path $taskRoot 'launcher'
$build=Join-Path $project ('build/'+(Get-Date -Format 'yyyyMMdd-HHmmssfff'))
$private=Join-Path $project 'private'
New-Item -ItemType Directory -Force -Path $build,(Join-Path $build 'gen'),(Join-Path $build 'classes'),(Join-Path $build 'dex'),$private | Out-Null
function Run([string]$Executable,[string[]]$Arguments) {
    & $Executable @Arguments
    if($LASTEXITCODE -ne 0){throw "Build failed: $Executable"}
}
if(!(Test-Path -LiteralPath (Join-Path $project 'assets/runtime.tar'))) { throw '先にpackage-setup.pyで同梱ランタイムを作成してください。' }
Run (Join-Path $bt 'aapt.exe') @('package','-f','-m','-M',(Join-Path $project 'AndroidManifest.xml'),'-S',(Join-Path $project 'res'),'-A',(Join-Path $project 'assets'),'-0','tar','-I',$android,'-F',(Join-Path $build 'unsigned.apk'),'-J',(Join-Path $build 'gen'))
$sources=@(Get-ChildItem (Join-Path $project 'src') -Recurse -Filter '*.java' | ForEach-Object FullName)
Run (Join-Path $jdk 'bin/javac.exe') (@('-encoding','UTF-8','-source','8','-target','8','-classpath',$android,'-d',(Join-Path $build 'classes'))+$sources)
Run (Join-Path $jdk 'bin/jar.exe') @('cf',(Join-Path $build 'classes.jar'),'-C',(Join-Path $build 'classes'),'.')
Run (Join-Path $bt 'd8.bat') @('--lib',$android,'--min-api','28','--output',(Join-Path $build 'dex'),(Join-Path $build 'classes.jar'))
Push-Location (Join-Path $build 'dex')
try { Run (Join-Path $bt 'aapt.exe') @('add',(Join-Path $build 'unsigned.apk'),'classes.dex') } finally { Pop-Location }
Run (Join-Path $bt 'zipalign.exe') @('-f','4',(Join-Path $build 'unsigned.apk'),(Join-Path $build 'aligned.apk'))
$key=Join-Path $private 'gemma-local.jks'
if($KeyStore) { $key=$KeyStore; if(!(Test-Path -LiteralPath $key)) { throw '指定した署名鍵がありません。' } }
if(!(Test-Path -LiteralPath $key)) {
    Run (Join-Path $jdk 'bin/keytool.exe') @('-genkeypair','-keystore',$key,'-storepass','android','-keypass','android','-alias','gemma','-dname','CN=Gemma Local','-keyalg','RSA','-keysize','2048','-validity','10000')
}
$apk=Join-Path $build 'Gemma.apk'
Run (Join-Path $bt 'apksigner.bat') @('sign','--ks',$key,'--ks-key-alias','gemma','--ks-pass','pass:android','--out',$apk,(Join-Path $build 'aligned.apk'))
Run (Join-Path $bt 'apksigner.bat') @('verify',$apk)
$report=[ordered]@{apk=$apk;sha256=(Get-FileHash -LiteralPath $apk -Algorithm SHA256).Hash.ToLower();bytes=(Get-Item -LiteralPath $apk).Length}
$report | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $project 'build/latest.json') -Encoding utf8
$report | ConvertTo-Json -Compress
