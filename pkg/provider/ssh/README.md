# SSH Provider

The SSH Provider extends the platform-health server to enable monitoring of SSH services. It performs protocol-level handshake(s) to capture host key fingerprints for security verification, without requiring authentication credentials. This enables MITM detection through host key fingerprint pinning and algorithm compliance checks using CEL (Common Expression Language) expressions.

When `algorithms` is specified, the provider polls each algorithm separately, collecting all fingerprints into a map for comprehensive verification.

## Usage

Once the SSH Provider is configured, any query to the platform health server will trigger validation of the configured SSH service(s). The server will attempt to establish SSH connection(s) to each instance, capture the host key(s) during the handshake, and report each instance as "healthy" if the handshake is successful and all CEL checks pass, or "unhealthy" if the connection fails or any check fails.

### Ad-hoc Check

```bash
# Basic SSH check (server chooses algorithm)
ph check ssh --host example.com --port 22

# Check specific algorithm
ph check ssh --host example.com --algorithms ssh-ed25519

# Check multiple algorithms
ph check ssh --host example.com --algorithms ssh-ed25519 --algorithms ecdsa-sha2-nistp256

# Check with CEL expression
ph check ssh --host example.com --check='"ssh-ed25519" in ssh.hostKey'
```

### Context Inspection

Use `ph context` to inspect the available CEL variables before writing expressions:

```bash
# Default (server chooses algorithm)
ph context ssh --host example.com

# Specific algorithm
ph context ssh --host example.com --algorithms ssh-ed25519

# Multiple algorithms
ph context ssh --host example.com --algorithms ssh-ed25519 --algorithms ecdsa-sha2-nistp256
```

## Configuration

The SSH Provider is configured through the platform-health server's configuration file. Each instance is defined with its name as the YAML key under `components`.

- `type` (required): Must be `ssh`.
- `spec`: Provider-specific configuration:
  - `host` (required): The hostname or IP address of the SSH service to monitor.
  - `port` (default: `22`): The port number of the SSH service to monitor.
  - `algorithms` (optional): List of host key algorithms to poll. If not specified, server chooses (single handshake). If specified, one handshake per algorithm.
- `checks`: A list of CEL expressions to validate the SSH connection. Each check has:
  - `check` (required): A CEL expression that must evaluate to `true` for the connection to be healthy.
  - `message` (optional): Custom error message when the check fails.

### Valid Algorithm Names

- `ssh-ed25519`
- `ecdsa-sha2-nistp256`
- `ecdsa-sha2-nistp384`
- `ecdsa-sha2-nistp521`
- `ssh-rsa`
- `rsa-sha2-256`
- `rsa-sha2-512`

## CEL Check Context

The SSH provider exposes an `ssh` variable containing host key details:

- `ssh.hostKey`: Map of algorithm name to SHA256 fingerprint (map[string]string)
- `ssh.host`: Target hostname used for connection (string)
- `ssh.port`: Target port used for connection (int)

### Example Context Output

Default (server chooses algorithm):
```json
{
  "ssh": {
    "hostKey": {
      "ecdsa-sha2-nistp256": "SHA256:p2QAMXNIC1TJYWeIOttrVc98/R1BUFWu3/LiyKgUfQM"
    },
    "host": "github.com",
    "port": 22
  }
}
```

With multiple algorithms specified:
```json
{
  "ssh": {
    "hostKey": {
      "ssh-ed25519": "SHA256:+DiY3wvvV6TuJJhbpZisF/zLDA0zPMSvHdkr4UvCOqU",
      "ecdsa-sha2-nistp256": "SHA256:p2QAMXNIC1TJYWeIOttrVc98/R1BUFWu3/LiyKgUfQM"
    },
    "host": "github.com",
    "port": 22
  }
}
```

### Example CEL Expressions

```cel
// Check if server supports specific algorithm
"ssh-ed25519" in ssh.hostKey

// Verify known host key fingerprint for specific algorithm (MITM detection)
ssh.hostKey["ssh-ed25519"] == "SHA256:+DiY3wvvV6TuJJhbpZisF/zLDA0zPMSvHdkr4UvCOqU"

// Check at least one modern algorithm is available
"ssh-ed25519" in ssh.hostKey || "ecdsa-sha2-nistp256" in ssh.hostKey

// Ensure RSA is not the only option (when polling multiple)
!("ssh-rsa" in ssh.hostKey) || size(ssh.hostKey) > 1
```

## Examples

### Basic SSH Check

```yaml
components:
  bastion:
    type: ssh
    spec:
      host: bastion.example.com
      port: 22
```

In this example, the SSH Provider will establish an SSH connection to `bastion.example.com` on port 22 and report the service as "healthy" if the handshake completes successfully. The server chooses which host key algorithm to present.

### Host Key Verification (Single Algorithm)

```yaml
components:
  production-ssh:
    type: ssh
    spec:
      host: prod.example.com
      port: 22
      algorithms:
        - ssh-ed25519
    checks:
      - check: 'ssh.hostKey["ssh-ed25519"] == "SHA256:uNiVztksCsDhcc0u9e8BujQXVUpKZIDTMczCvj3tD2s"'
        message: "Host key mismatch - possible MITM attack"
```

This configuration verifies that the SSH server presents the expected ED25519 host key, providing protection against man-in-the-middle attacks.

### Multi-Algorithm Verification

```yaml
components:
  secure-bastion:
    type: ssh
    spec:
      host: bastion.example.com
      algorithms:
        - ssh-ed25519
        - ecdsa-sha2-nistp256
    checks:
      - check: '"ssh-ed25519" in ssh.hostKey'
        message: "Server must support ED25519"
      - check: 'ssh.hostKey["ssh-ed25519"] == "SHA256:expected..."'
        message: "ED25519 key mismatch"
      - check: 'ssh.hostKey["ecdsa-sha2-nistp256"] == "SHA256:expected..."'
        message: "ECDSA key mismatch"
```

This polls both ED25519 and ECDSA keys and verifies both fingerprints.

### Algorithm Compliance Check

```yaml
components:
  modern-ssh:
    type: ssh
    spec:
      host: secure.example.com
      algorithms:
        - ssh-ed25519
    checks:
      - check: '"ssh-ed25519" in ssh.hostKey'
        message: "ED25519 host key required"
```

This ensures the SSH server supports the modern ED25519 algorithm. The check will fail if the server doesn't support ED25519.
