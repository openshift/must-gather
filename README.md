must-gather
===========

`openshift-must-gather` is a tool for collecting cluster data.
It dumps `clusteroperator` data, and associated namespace data, into a specified `--base-dir` location.
The directory structure, as well as specific details behind this tool can be found [in this enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/oc/must-gather.md).

## Collection Scripts
Data collection scripts are kept in `./collection-scripts`.  The content of that folder is placed in `/usr/bin` in the image.
The data collection scripts should only include collection logic for components that are included as part of the OpenShift
CVO payload.  Outside components are encouraged to produce a similar "must-gather" image, but this is not the spot to be
included.

## How to run
You can run `must-gather` on a cluster with `oc adm must-gather`. 
Use `-h` flag to see available options.

## How to test
Build a new must-gather image with your changes using the Makefile. There are two options in the Makefile, `make` will use imagebuilder to build the image while `make BUILDER=podman` will use podman to build the image.

If using `make BUILDER=podman`, please set the `AUTH_FILE` parameter pointing to an authentication file that has credentials for the `registry.ci.openshift.org` registry. Ensure that you are connected to the VPN before building the image.
```
make BUILDER=podman AUTH_FILE=/path/to/authfile
```

Once the image is built, push it to a registry where it can be accessed from. 
```
podman push [built-image] [registry/username/your-built-image]
``` 

To test your new changes with a cluster, run the following:
```
oc adm must-gather --image=registry/username/your-built-image
```

## Obfuscate confidential information
There is a dedicated effort to obfuscate and omit confidential information. Head over to [openshift/must-gather-clean](https://github.com/openshift/must-gather-clean) for more information.

## Loki Log Collection

### Options
- `--only-loki` - Collect only Loki logs (skip traditional must-gather)
- `--logs-namespace=NAME` - Target specific namespace
- `LOKI_MAX_BATCHES=N` - Batches per log type (default: 100, each batch contain 5000 logs)

### Output Structure
```
loki-logs/
├── application_logs_part1.txt     (250K records)
├── application_logs_part2.txt     (remaining records)
├── infrastructure_logs_part1.txt  (250K records)
├── audit_logs_part1.txt           (250K records)
└── metadata/
    ├── clusterlogging.yaml
    ├── loki_routes.yaml
    └── loki_pods.yaml
```
