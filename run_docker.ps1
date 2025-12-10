# Set the execution preference to stop on non-terminating errors, similar to 'set -e'
$ErrorActionPreference = "Stop"

# --- 1. Define and Check .env File Path ---

# Get the absolute path to the .env file, which is expected to be in the same directory as the script.
$EnvFilePath = Join-Path $PSScriptRoot ".env"

if (-not (Test-Path -Path $EnvFilePath -PathType Leaf)) {
    Write-Error "The .env file was not found at '$EnvFilePath'!"
    exit 1
}

# --- (Optional) Connecting to a Service in Kubernetes ---

# If the OPENWEBUIHOSTURL points to a service running inside a Kubernetes cluster,
# you must forward the service's port to your local machine before running this script.
#
# 1. Find the service name and port in Kubernetes (e.g., 'open-webui' on port 8080).
#    kubectl get service -n <your-namespace>
#
# 2. In a SEPARATE terminal, run kubectl port-forward. This is a blocking command.
#    # kubectl port-forward service/<service-name> <local-port>:<service-port>
#    kubectl port-forward service/open-webui 9090:8080
#
# 3. Update the OPENWEBUIHOSTURL in your .env file to point to the local port.
#    OPENWEBUIHOSTURL=http://localhost:9090

# --- 2. Prepare the Environment Variables for Docker ---

Write-Host "Reading environment variables from .env file..."

# This array will hold the alternating '-e' and 'KEY=VALUE' arguments for the docker run command
$DockerEnvArguments = @()
$FoundKeys = @{}

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

            if (-not $Value) {
                Write-Error "The environment variable '$Key' in '$EnvFilePath' must not have an empty value."
                exit 1
            }
            $FoundKeys[$Key] = $true
        }
    }
}

# Validate that all required keys were found
$MissingKeys = $RequiredKeys | Where-Object { -not $FoundKeys.ContainsKey($_) }

if ($MissingKeys) {
    Write-Error "The following required environment variables were not found in '$EnvFilePath': $($MissingKeys -join ', ')"
    exit 1
}



# --- 3. Build the Docker container ---

Write-Host "`nBuilding Docker image 'go-agent-api'..."
docker build -t go-agent-api .

if ($LASTEXITCODE -ne 0) {
    Write-Error "Docker build failed (exit code $LASTEXITCODE). Aborting container run."
    exit 1
}

# --- 4. Run the Docker container ---

$ContainerName = "go-agent-api-container"

# Stop and remove the container if it already exists to prevent conflicts
Write-Host "`nStopping and removing old container instance '$ContainerName'..."
docker rm -f $ContainerName 2>$null

Write-Host "Starting new container '$ContainerName' on port 8080..."

# Construct the full argument list for docker run
$RunArguments = @(
    "run",
    "--name", $ContainerName,
    "-p", "8080:8080",
    # Add '--rm' to automatically clean up the container when it exits
    "--rm"
)

# Add the environment variables arguments
$RunArguments += $DockerEnvArguments

# Add the image name
$RunArguments += "go-agent-api"

# Execute the final command using the call operator (&)
try {
    & docker $RunArguments
} catch {
    Write-Error "Failed to run Docker container. Error: $_"
    exit 1
}