# Port Forwarder
A small program to port forward

## What is this?

This a small program to forward ports.
Some features of this code are:
* **Easy to use**: Configuration file uses YAML syntax
* **High performance**: I did not re-test. It should be the same, if not faster, than the base project as we have less overhead.
* **Simultaneous Conn Limit**: Limit the amount of simultaneous _connections_ that a port can have.

## Install

Just head to [releases](https://github.com/Taulim/GoPortForward/releases) and download one for your os.

### Build

Building this is not that hard. At first install [golang](https://golang.org/dl/) for your operating system. Clone this repository and run this command:
```bash
go build
```
The executable file will be available at the present directory.

## How to use it?

Did you download the executable for your os? Good!

Edit the `rules.yaml` file as you wish. Here is the cheatsheet for it:
* `Timeout`: The time in seconds that a connection can stay alive without transmitting any data. Default is disabled. Use 0 to disable the timeout.
* `Rules` Each element represents a forwarding rule and quota for it.
    * `Name`: Just a name for this rule. It does not have any effect on the application.
    * `Listen`: The local port to accept the incoming connections for proxy.
    * `Forward`: The address that the traffic must be forwarded to. Enter it like `ip:port`
    * `Simultaneous`: Amount of allowed simultaneous connections to this port. Use 0, or remove it for unlimited.
    
Save the file and just open the main executable to run the proxy.

### Arguments

There are two options:
1. `-h`: It prints out the help of the proxy.
3. `--verbose`: Verbose mode (a number between 0 and 4)
4. `--config`: In case you want to use a config file with another name, just pass it to program as the first argument. For example:
```bash
./GoPortForward --config custom_conf.json
```

#### Verbose modes

You have 5 options

- `0` Fatal errors.
- `1` Regular errors and information. (This is the default)
- `2` Same as above and connection limit.
- `3` Sane as above and connection timeout.
- `4` Everything.

Example:
```bash
./GoPortForward --verbose 2
```

## How it works?

This project is based on the [PortFowrwarder](https://github.com/HirbodBehnam/PortForwarder).  
It works in the same way, but has less features (less code and less overhead).  
Also uses the more user friendly YAML for configuration.
