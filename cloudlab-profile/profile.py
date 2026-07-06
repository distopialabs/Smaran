"""CloudLab profile for Smaran artifact evaluation.

Provisions two nodes matching the paper's experimental setup:
- node0 (server): r6615 at Clemson  (Ubuntu 22.04)
- node1 (client): c6420 at Clemson  (Ubuntu 22.04)

Both nodes share a LAN and can reach each other by short hostname
(`node0`, `node1`).

The profile has two variants, selectable at instantiate time:

- `Pre-built image` (recommended for AE): boots from a snapshotted disk
  image with Smaran / OPTIKS / CONIKS already installed. Fill in
  `IMAGE_URN` below after snapshotting the first successful build.

- `From source`: boots a clean Ubuntu 22.04 image. The evaluator then
  runs the scripts under KeyTransparencyScripts/ to install everything.

Usage:
    geni-lib profile --project distopialabs profile.py

Or upload directly at:
    https://www.cloudlab.us/manage_profile.php
"""
import geni.portal as portal
import geni.rspec.pg as pg

pc = portal.Context()

# Fill this in AFTER you snapshot a working install. Placeholder URN below.
# Format: urn:publicid:IDN+utah.cloudlab.us+image+<project>//<image-name>:<version>
IMAGE_URN_PLACEHOLDER = "urn:publicid:IDN+utah.cloudlab.us+image+distopialabs-PG0//smaran-ae:0"
CLEAN_UBUNTU_URN = "urn:publicid:IDN+emulab.net+image+emulab-ops//UBUNTU22-64-STD"

pc.defineParameter(
    "useImage",
    "Boot from the pre-built Smaran image (recommended)",
    portal.ParameterType.BOOLEAN,
    True,
)
pc.defineParameter(
    "imageURN",
    "Disk image URN (only used if 'useImage' is checked)",
    portal.ParameterType.STRING,
    IMAGE_URN_PLACEHOLDER,
)
pc.defineParameter(
    "serverHW", "Hardware type for the server node",
    portal.ParameterType.NODETYPE, "r6615",
)
pc.defineParameter(
    "clientHW", "Hardware type for the client node",
    portal.ParameterType.NODETYPE, "c6420",
)
params = pc.bindParameters()

rspec = pg.Request()

disk_image = params.imageURN if params.useImage else CLEAN_UBUNTU_URN

server = pg.RawPC("node0")
server.hardware_type = params.serverHW
server.disk_image = disk_image
rspec.addResource(server)

client = pg.RawPC("node1")
client.hardware_type = params.clientHW
client.disk_image = disk_image
rspec.addResource(client)

lan = pg.LAN("lan0")
lan.addInterface(server.addInterface("if0"))
lan.addInterface(client.addInterface("if0"))
rspec.addResource(lan)

# If booting from a clean image, seed the repo checkout so the evaluator can
# immediately cd ~/Smaran and run the installers.
if not params.useImage:
    bootstrap = (
        "sudo apt-get update -qq && "
        "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git && "
        "cd $HOME && "
        "git clone --recurse-submodules -b artifact-eval "
        "https://github.com/distopialabs/Smaran.git Smaran || true"
    )
    server.addService(pg.Execute(shell="bash", command=bootstrap))
    client.addService(pg.Execute(shell="bash", command=bootstrap))

pc.printRequestRSpec(rspec)
