# vultr-cloudinit
Application that converts Vultr Metadata to Cloud-Init NoCloud compatible data

## Requirements

* The ability to set Cloud-Init User-Data when creating instances
  * This feature is currently in closed beta and may not be available to all accounts

* Currently this only supports IPv4 networks

## Installation

This installation guide assumes your instance is linux based with systemd.

1. Install cloud-init on your instance
1. Download the latest `vultr-cloudinit`
1. Make the seed directory
    ```
    mkdir -p /var/lib/cloud/seed/nocloud/
    ```
1. Allow persistent rules to be set `chattr -i /etc/udev/rules.d/70-persistent-net.rules`
1. Make the systemd override
    ```
    mkdir -p /etc/systemd/system/cloud-init-local.service.d/
    cat > /etc/systemd/system/cloud-init-local.service.d/10-vultr.conf << EOF
    [Service]
    ExecStartPre=/usr/local/bin/vultr-cloudinit -o /var/lib/cloud/seed/nocloud/
    EOF
    ```
1. Configure Cloud-Init
    ```
    cat >> /etc/cloud/cloud.cfg << EOF
    datasource:
      NoCloud:
        seedfrom: /var/lib/cloud/seed/nocloud/
        meta-data:
          instance-id: iid-local01
          local-hostname: firstboot.cloud
    EOF
    cat > /etc/cloud/cloud.cfg.d/91-dib-cloud-init-datasources.cfg <<EOF
    datasource_list: [ NoCloud ]
    EOF
    ```
1. Reboot the instance and it should now be using cloud-init