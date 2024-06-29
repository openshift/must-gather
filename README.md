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

## Obfuscate confidential information
There is a dedicated effort to obfuscate and omit confidential information. Head over to [openshift/must-gather-clean](https://github.com/openshift/must-gather-clean) for more information.
