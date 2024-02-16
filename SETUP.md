# Setting Up a Benchmarking Node

This guide describes the process of setting up a HostScore benchmarking node.

## Prerequisites

You will need a server with the root access over SSH. The blockchain is quite large (around 50GB at the moment of writing), so you need to account for that. It is recommended to use an SSD, because it will affect the syncing speed.

This guide will assume that you use Ubuntu Server 22.04 LTS. If you run any other OS, the setup process may differ.

## Downloading Binaries

Log into your server and download the binaries. This guide assumes that you will use the version `0.1.0` for an x86 CPU:
```
mkdir ~/hostscore
cd ~/hostscore
wget -q https://github.com/mike76-dev/hostscore/releases/download/v0.1.0/hostscore_linux_amd64.zip
unzip hostscore_linux_amd64.zip
rm hostscore_linux_amd64.zip
```

## Setting Up the Firewall

You need to configure the firewall to allow outside access to the API port from the `hsc` client only.

List the `ufw` application profiles by running the following:
```
$ sudo ufw app list
```
Your output will be a list of the application profiles:
```
Output:
Available applications:
  OpenSSH
```
If you haven't done so yet, allow SSH connections to your server:
```
$ sudo ufw allow 'OpenSSH'
```
And enable your firewall:
```
$ sudo ufw enable
```
Now you need to allow incoming connections to ports `9980` and `9880` from the IP address of the client machine where `hsc` will be running:
```
$ sudo ufw allow from <IP_address> proto tcp to any port 9980
$ sudo ufw allow from <IP_address> proto tcp to any port 9880
```
You can verify the change by checking the status:
```
$ sudo ufw status
```
The output will provide a list of allowed HTTP traffic:
```
Output:
Status: active

To                         Action      From
--                         ------      ----
OpenSSH                    ALLOW       Anywhere
9880/tcp                   ALLOW       <IP_address>
9980/tcp                   ALLOW       <IP_address>
OpenSSH (v6)               ALLOW       Anywhere (v6)
```

## Installing MySQL

`hsd` uses a MySQL database to store the metadata. To install MySQL on your server, run this:
```
$ sudo apt install mysql-server
```
Ensure that the server is running using the `systemctl start` command:
```
$ sudo systemctl start mysql
```
These commands will install and start MySQL, but will not prompt you to set a password or make any other configuration changes. Because this leaves your installation of MySQL insecure, we will address this next. First, open up the MySQL prompt:
```
$ sudo mysql
```
Then run the following `ALTER USER` command to change the root user’s authentication method to one that uses a password. The following example changes the authentication method to `mysql_native_password`:
```
mysql> ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY 'password';
```
After making this change, exit the MySQL prompt:
```
mysql> exit;
```
Run the security script with `sudo`:
```
$ sudo mysql_secure_installation
```
This will take you through a series of prompts where you can make some changes to your MySQL installation’s security options. The first prompt will ask whether you’d like to set up the Validate Password Plugin, which can be used to test the password strength of new MySQL users before deeming them valid.

If you elect to set up the Validate Password Plugin, any MySQL user you create that authenticates with a password will be required to have a password that satisfies the policy you select:
```
Output:
Securing the MySQL server deployment.

Connecting to MySQL using a blank password.

VALIDATE PASSWORD COMPONENT can be used to test passwords
and improve security. It checks the strength of password
and allows the users to set only those passwords which are
secure enough. Would you like to setup VALIDATE PASSWORD component?

Press y|Y for Yes, any other key for No: Y
```
```
Output:
There are three levels of password validation policy:

LOW    Length >= 8
MEDIUM Length >= 8, numeric, mixed case, and special characters
STRONG Length >= 8, numeric, mixed case, special characters and dictionary file

Please enter 0 = LOW, 1 = MEDIUM and 2 = STRONG:
 2
```
If you used the Validate Password Plugin, you’ll receive feedback on the strength of your new password. Then the script will ask if you want to continue with the password you just entered or if you want to enter a new one. Assuming you’re satisfied with the strength of the password you just entered, enter `Y` to continue the script:
```
Output:
Estimated strength of the password: 100
Do you wish to continue with the password provided?(Press y|Y for Yes, any other key for No) : Y
```
From there, you can press `Y` and then `ENTER` to accept the defaults for all the subsequent questions. This will remove some anonymous users and the test database, disable remote root logins, and load these new rules so that MySQL immediately respects the changes you have made.

Once the security script completes, you can then reopen MySQL and change the root user’s authentication method back to the default, `auth_socket`. To authenticate as the root MySQL user using a password, run this command:
```
$ mysql -u root -p
```
Then go back to using the default authentication method using this command:
```
mysql> ALTER USER 'root'@'localhost' IDENTIFIED WITH auth_socket;
```
This will mean that you can once again connect to MySQL as your root user using the `sudo mysql` command.
```
$ sudo mysql
```
Allow the root user to grant privileges:
```
mysql> GRANT ALL PRIVILEGES ON *.* TO 'root'@'localhost' WITH GRANT OPTION;
mysql> FLUSH PRIVILEGES;
```
Run the following command to create a database for HostScore. This guide will be using `hostscore` as the database name. Take a note of this name:
```
mysql> CREATE DATABASE hostscore;
```
Then create a user for this database. This guide will be using `hsuser` as the user name. Take a note of this name and be sure to change password to a strong password of your choice:
```
mysql> CREATE USER 'hsuser'@'localhost' IDENTIFIED WITH mysql_native_password BY 'password';
```
Now grant this user access to the database:
```
mysql> GRANT ALL PRIVILEGES ON hostscore.* TO 'hsuser'@'localhost';
mysql> FLUSH PRIVILEGES;
```
Exit MySQL and log in as `hsuser`:
```
mysql> exit;
$ cd ~/hostscore
$ mysql -u hsuser -p
```
Create the database tables:
```
mysql> USE hostscore;
mysql> SOURCE init.sql;
```
Now exit MySQL:
```
mysql> exit;
```

## Configuring HSD

Create the config directories:
```
sudo mkdir /usr/local/etc/hsd-mainnet
sudo mkdir /usr/local/etc/hsd-zen
sudo chown <user> /usr/local/etc/*
```
where `user` is the name of the user that will be running the service.

Open the `hsdconfig.json` file:
```
$ nano hsdconfig.json
```
First, choose a `name` of your hsd node. Fill in the `dbUser` and `dbName` fields with the MySQL user name (`hsuser`) and the database name (`hostscore`). Set the directory to store the `hsd` metadata and log files (here it is `/usr/local/etc/hsd-mainnet`). You can also change the default port numbers:
```
"HSD Configuration"
"0.1.0"
{
        "name": "",
        "gateway": ":9981",
        "api": ":9980",
        "dir": "/usr/local/etc/hsd-mainnet",
        "dbUser": "hsuser",
        "dbName": "hostscore",
        "network": "mainnet"
}
```
Save and exit. Now copy the file to its new location:
```
$ cp hsdconfig.json /usr/local/etc/hsd-mainnet
```
Now repeat the same for the Zen network. Note that the port numbers are different:
```
$ nano hsdconfig.json
```
```
"HSD Configuration"
"0.1.0"
{
        "name": "",
        "gateway": ":9881",
        "api": ":9880",
        "dir": "/usr/local/etc/hsd-zen",
        "dbUser": "hsuser",
        "dbName": "hostscore",
        "network": "zen"
}
```
```
$ cp hsdconfig.json /usr/local/etc/hsd-zen
```
Now copy the `hsd` binary over:
```
$ sudo cp hsd /usr/local/bin
```
Generate a new wallet seed:
```
$ hsd seed
```
```
Output:
Seed:    belt thought dignity indoor find judge field foot next robot impose layer
Address: 5ea89aa7dd4a8f7db0bb9d5a7e9f11f2e141db9b964158c20e24462386b3925462b733f2fc44
```
You will be using the same seed for both the Mainnet and the Zen network. Take a note of it as well as of the generated wallet address. Fund this address with Siacoin on both networks.

The easiest way to run `hsd` is via `systemd`. You need to create two service file, one for each network:
```
$ sudo nano /etc/systemd/system/hsd-mainnet.service
```
Enter the following lines. Replace:
`<user>` with the name of the user that will be running `hsd`,
`<api_password>` with the `hsd` API password of your choice,
`<db_password>` with the MySQL user password created earlier,
`<wallet_seed>` with the wallet seed generated at the previous step,
```
[Unit]
Description=hsd-mainnet
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/hsd --dir=/usr/local/etc/hsd-mainnet
TimeoutStopSec=660
Restart=always
RestartSec=15
User=<user>
Environment="HSD_API_PASSWORD=<api_password>"
Environment="HSD_DB_PASSWORD=<db_password>"
Environment="HSD_WALLET_SEED=<wallet_seed>"
Environment="HSD_CONFIG_DIR=/usr/local/etc/hsd-mainnet"
LimitNOFILE=900000

[Install]
WantedBy=multi-user.target
Alias=hsd-mainnet.service
```
Save and exit. Repeat the same for the Zen network:
```
$ sudo nano /etc/systemd/system/hsd-zen.service
```
```
[Unit]
Description=hsd-zen
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/hsd --dir=/usr/local/etc/hsd-zen
TimeoutStopSec=660
Restart=always
RestartSec=15
User=<user>
Environment="HSD_API_PASSWORD=<api_password>"
Environment="HSD_DB_PASSWORD=<db_password>"
Environment="HSD_WALLET_SEED=<wallet_seed>"
Environment="HSD_CONFIG_DIR=/usr/local/etc/hsd-zen"
LimitNOFILE=900000

[Install]
WantedBy=multi-user.target
Alias=hsd-zen.service
```
Now you are ready to start `hsd`:
```
$ sudo systemctl start hsd-mainnet
$ sudo systemctl start hsd-zen
```
Open the `systemd` journal to see the log:
```
$ journalctl -u hsd-zen -f
```
If everything went well, you should see the following output:
```
Output:
Feb 16 09:41:40 server systemd[1]: Started hsd-zen.
Feb 16 09:41:40 server hsd[1330]: Using HSD_CONFIG_DIR environment variable to load config.
Feb 16 09:41:40 server hsd[1330]: Using HSD_API_PASSWORD environment variable.
Feb 16 09:41:40 server hsd[1330]: Using HSD_DB_PASSWORD environment variable.
Feb 16 09:41:40 server hsd[1330]: Using HSD_WALLET_SEED environment variable.
Feb 16 09:41:40 server hsd[1330]: hsd v0.1.0
Feb 16 09:41:40 server hsd[1330]: Git Revision 047f00c
Feb 16 09:41:40 server hsd[1330]: Loading...
Feb 16 09:41:40 server hsd[1330]: Connecting to the SQL database...
Feb 16 09:41:40 server hsd[1330]: p2p: Listening on [::]:9881
Feb 16 09:41:40 server hsd[1330]: api: Listening on 127.0.0.1:9880
```
The daemon will now be syncing to the blockchain. You can monitor the progress with the following command:
```
$ curl -u "":<api_password> http://localhost:9880/api/consensus/tip
```
```
Output:
{
	"height": 29110,
	"id": "bid:0000000179cbc819da2cf4791d3c0ffa1d36f8e0ed1590e6811bbfcea88aafe3",
	"synced": false
}
```
Once the node is synced, the output will change:
```
Output:
{
	"height": 57444,
	"id": "bid:00000003609a4a84c93c50574410cf4457ce291a065148aa389899982c01d089",
	"synced": true
}
```
Check that the wallet has funds in it:
```
$ curl -u "":<api_password> http://localhost:9880/api/wallet/balance
```
```
Output:
{
	"siacoins": "50000000000000000000000000000",
	"immatureSiacoins": "0",
	"siafunds": 0
}
```
`hsd` will start forming contracts with the hosts and benchmarking them. You are all set!
