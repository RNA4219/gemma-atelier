param(
    [Parameter(Mandatory=$true)][string]$SdkRoot,
    [string]$NdkRoot = "$env:LOCALAPPDATA\Android\Sdk\ndk\29.0.13113456",
    [Parameter(Mandatory=$true)][string]$Python,
    [int]$Jobs = 6
)
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$source = "$taskRoot/engine"
$env:TEMP = "$taskRoot/build/tmp"
$env:TMP = $env:TEMP
New-Item -ItemType Directory -Path $env:TEMP -Force | Out-Null
$sdk = (Resolve-Path -LiteralPath $SdkRoot).Path.Replace('\', '/')
$sdkMeta = Get-Content -LiteralPath "$sdk/hexagon_sdk.json" -Raw | ConvertFrom-Json
$toolsRoot = "$sdk/$($sdkMeta.root.tools.info[0].path)"
if (-not (Test-Path -LiteralPath "$toolsRoot/Tools/bin/hexagon-clang.exe")) { throw 'Hexagon compiler missing' }
$dspBuild = "$taskRoot/build/htp-v75"
& cmake -S "$source/ggml/src/ggml-hexagon/htp" -B $dspBuild -G Ninja `
    "-DCMAKE_TOOLCHAIN_FILE=$source/ggml/src/ggml-hexagon/htp/cmake-toolchain.cmake" `
    "-DHEXAGON_SDK_ROOT=$sdk" "-DHEXAGON_TOOLS_ROOT=$toolsRoot" `
    "-DPython3_EXECUTABLE=$Python" -DDSP_VERSION=v75 -DPREBUILT_LIB_DIR=toolv19_v75 -DCMAKE_BUILD_TYPE=Release
if ($LASTEXITCODE -ne 0) { throw 'DSP configure failed' }
& cmake --build $dspBuild -j $Jobs
if ($LASTEXITCODE -ne 0) { throw 'DSP build failed' }
$hostSdk = "$taskRoot/build/host-sdk"
foreach ($dir in @('incs', 'generated', 'lib', 'utils/examples')) {
    New-Item -ItemType Directory -Path "$hostSdk/$dir" -Force | Out-Null
}
Copy-Item -Path "$sdk/incs/*" -Destination "$hostSdk/incs" -Recurse -Force
if (Test-Path -LiteralPath "$sdk/utils/examples") {
    Copy-Item -Path "$sdk/utils/examples/*" -Destination "$hostSdk/utils/examples" -Recurse -Force
}
Copy-Item -LiteralPath "$dspBuild/htp_iface.h", "$dspBuild/htp_iface_stub.c" -Destination "$hostSdk/generated" -Force
Copy-Item -LiteralPath "$dspBuild/libggml-htp-v75.so" -Destination "$hostSdk/lib" -Force
$build = "$taskRoot/build/android"
& cmake -S $source -B $build -G Ninja `
    "-DCMAKE_TOOLCHAIN_FILE=$NdkRoot/build/cmake/android.toolchain.cmake" `
    -DANDROID_ABI=arm64-v8a -DANDROID_PLATFORM=android-28 -DANDROID_STL=c++_static `
    -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF -DGGML_NATIVE=OFF `
    -DGGML_HEXAGON=ON "-DGGML_HEXAGON_PREBUILT_ROOT=$hostSdk" `
    -DGGML_OPENMP=OFF -DGGML_LLAMAFILE=OFF -DLLAMA_OPENSSL=OFF `
    -DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_EXAMPLES=OFF `
    -DLLAMA_BUILD_SERVER=ON -DLLAMA_BUILD_APP=OFF -DGGML_CPU_KLEIDIAI=OFF `
    -DLLAMA_USE_PREBUILT_UI=OFF
if ($LASTEXITCODE -ne 0) { throw 'Android configure failed' }
& cmake --build $build --target llama-completion llama-server -j $Jobs
if ($LASTEXITCODE -ne 0) { throw 'Android build failed' }
$deploy = "$taskRoot/build/deploy"
New-Item -ItemType Directory -Path $deploy -Force | Out-Null
$strip = "$NdkRoot/toolchains/llvm/prebuilt/windows-x86_64/bin/llvm-strip.exe"
foreach ($binary in @('llama-completion', 'llama-server')) {
    & $strip --strip-unneeded "$build/bin/$binary" -o "$deploy/$binary"
    if ($LASTEXITCODE -ne 0) { throw "strip failed: $binary" }
}
Copy-Item -LiteralPath "$dspBuild/libggml-htp-v75.so" -Destination $deploy -Force
Get-FileHash -Algorithm SHA256 -Path "$deploy/*" | Format-Table -AutoSize
