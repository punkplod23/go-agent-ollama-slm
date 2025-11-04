# Set the execution preference to stop on non-terminating errors, similar to 'set -e'
$ErrorActionPreference = "Stop"

# --- 1. Define and Check .env File Path ---

# Get the absolute path to the .env file, which is expected to be in the same directory as the script.
$EnvFilePath = Join-Path $PSScriptRoot ".env"

if (-not (Test-Path -Path $EnvFilePath -PathType Leaf)) {
    Write-Error "The .env file was not found at '$EnvFilePath'!"
    exit 1
}

# --- 2. Prepare the Environment Variables for Docker ---

Write-Host "Reading environment variables from .env file..."

# This array will hold the alternating '-e' and 'KEY=VALUE' arguments for the docker run command
$DockerEnvArguments = @()

# List of environment variables that need to be passed to the Docker container
$RequiredKeys = @(
    "OPENWEBUIHOSTURL", 
    "OPENWEBUIAPITOKEN", 
    "OPENWEBUIMODELNAME", 
    "DVSAAPIURL", 
    "OPENALPRAPIURL"
)

# Read the file content, filter out comments (#) and empty lines.
Get-Content $EnvFilePath | ForEach-Object {
    $Line = $_.Trim()
    
    # Skip comments and empty lines
    if ($Line -notmatch '^(#|$)') {
        # Split the line only at the first '=' to handle values that might contain '='
        $Parts = $Line.Split('=', 2)
        $Key = $Parts[0].Trim()
        $Value = $Parts[1].Trim()

        if ($RequiredKeys -contains $Key) {
            # Add the -e argument to the array, followed by the KEY=VALUE pair
            $DockerEnvArguments += "-e"
            $DockerEnvArguments += "$Key=$Value"
        }
    }
}

if ($DockerEnvArguments.Count -lt 14) {
    Write-Warning "Warning: Not all 7 expected DVSA variables were found in the .env file or successfully parsed."
}


# --- 3. Build the Docker container ---

Write-Host "`nBuilding Docker image 'go-agent-api'..."
docker build -t go-agent-api .

if ($LASTEXITCODE -ne 0) {
    Write-Error "Docker build failed (exit code $LASTEXITCODE). Aborting container run."
    exit 1
}

# --- 4. Run the Docker container ---

Write-Host "`nRunning Docker container 'go-agent-api' on port 8080..."

# Construct the full argument list for docker run
$RunArguments = @(
    "run",
    "-p", "8080:8080"
)

# Add the environment variables arguments
$RunArguments += $DockerEnvArguments

# Add the image name
$RunArguments += "go-agent-api"

# Execute the final command using the call operator (&)
& docker $RunArguments