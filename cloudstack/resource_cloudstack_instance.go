//
// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.
//

package cloudstack

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/apache/cloudstack-go/v2/cloudstack"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceCloudStackInstance() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudStackInstanceCreate,
		Read:   resourceCloudStackInstanceRead,
		Update: resourceCloudStackInstanceUpdate,
		Delete: resourceCloudStackInstanceDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceCloudStackInstanceImportContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"display_name": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"service_offering": {
				Type:     schema.TypeString,
				Required: true,
			},

			"network_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"ip_address": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"ip6_address": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"ip6_cidr": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"template": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"root_disk_size": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"group": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"affinity_group_ids": {
				Type:          schema.TypeSet,
				Optional:      true,
				Elem:          &schema.Schema{Type: schema.TypeString},
				Set:           schema.HashString,
				ConflictsWith: []string{"affinity_group_names"},
			},

			"affinity_group_names": {
				Type:          schema.TypeSet,
				Optional:      true,
				Elem:          &schema.Schema{Type: schema.TypeString},
				Set:           schema.HashString,
				ConflictsWith: []string{"affinity_group_ids"},
			},

			"security_group_ids": {
				Type:          schema.TypeSet,
				Optional:      true,
				ForceNew:      true,
				Elem:          &schema.Schema{Type: schema.TypeString},
				Set:           schema.HashString,
				ConflictsWith: []string{"security_group_names"},
			},

			"security_group_names": {
				Type:          schema.TypeSet,
				Optional:      true,
				ForceNew:      true,
				Elem:          &schema.Schema{Type: schema.TypeString},
				Set:           schema.HashString,
				ConflictsWith: []string{"security_group_ids"},
			},

			"project": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"zone": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"keypair": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"keypairs"},
			},

			"keypairs": {
				Type:          schema.TypeList,
				Optional:      true,
				Elem:          &schema.Schema{Type: schema.TypeString},
				ConflictsWith: []string{"keypair"},
			},

			"host_id": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"cluster_id": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"uefi": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"start_vm": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  true,
				ForceNew: true,
			},

			"user_data": {
				Type:     schema.TypeString,
				Optional: true,
				StateFunc: func(v interface{}) string {
					switch v.(type) {
					case string:
						hash := sha1.Sum([]byte(v.(string)))
						return hex.EncodeToString(hash[:])
					default:
						return ""
					}
				},
			},

			"details": {
				Type:     schema.TypeMap,
				Optional: true,
			},

			"properties": {
				Type:     schema.TypeMap,
				Optional: true,
			},

			"nicnetworklist": {
				Type:     schema.TypeMap,
				Optional: true,
			},

			"expunge": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"pod_id": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"tags": tagsSchema(),
		},
	}
}

func resourceCloudStackInstanceCreate(d *schema.ResourceData, meta interface{}) error {

	cs := meta.(*cloudstack.CloudStackClient)

	// Retrieve the service_offering ID
	serviceofferingid, e := retrieveID(cs, "service_offering", d.Get("service_offering").(string))
	if e != nil {
		return e.Error()
	}

	// Retrieve the zone ID
	zoneid, e := retrieveID(cs, "zone", d.Get("zone").(string))
	if e != nil {
		return e.Error()
	}

	// Retrieve the zone object
	zone, _, err := cs.Zone.GetZoneByID(zoneid)
	if err != nil {
		return err
	}

	// Retrieve the template ID
	templateid, e := retrieveTemplateID(cs, zone.Id, d.Get("template").(string))
	if e != nil {
		return e.Error()
	}

	// Create a new parameter struct
	p := cs.VirtualMachine.NewDeployVirtualMachineParams(serviceofferingid, templateid, zone.Id)
	p.SetStartvm(d.Get("start_vm").(bool))
	vmDetails := make(map[string]string)
	if details, ok := d.GetOk("details"); ok {
		for k, v := range details.(map[string]interface{}) {
			vmDetails[k] = v.(string)
		}
		p.SetDetails(vmDetails)
	}

	// Set VM Properties
	vmProperties := make(map[string]string)
	if properties, ok := d.GetOk("properties"); ok {
		for k, v := range properties.(map[string]interface{}) {
			vmProperties[k] = v.(string)
		}
		p.SetProperties(vmProperties)
	}

	// SetNicNetworkList
	if nicnetworklist, ok := d.GetOk("nicnetworklist"); ok {
		nicNetworkDetails := []map[string]string{
			{
				"nic":     nicnetworklist.(map[string]interface{})["nic"].(string),
				"network": nicnetworklist.(map[string]interface{})["network"].(string),
			},
		}
		p.SetNicnetworklist(nicNetworkDetails)
	}

	// Set the name
	name, hasName := d.GetOk("name")
	if hasName {
		p.SetName(name.(string))
	}

	// Set the display name
	if displayname, ok := d.GetOk("display_name"); ok {
		p.SetDisplayname(displayname.(string))
	} else if hasName {
		p.SetDisplayname(name.(string))
	}

	// If there is a root_disk_size supplied, add it to the parameter struct
	if rootdisksize, ok := d.GetOk("root_disk_size"); ok {
		p.SetRootdisksize(int64(rootdisksize.(int)))
	}

	if d.Get("uefi").(bool) {
		p.SetBoottype("UEFI")
		p.SetBootmode("Legacy")
	}

	if zone.Networktype == "Advanced" {
		// Set the default network ID
		p.SetNetworkids([]string{d.Get("network_id").(string)})
	}

	// If there is a ipaddres supplied, add it to the parameter struct
	if ipaddress, ok := d.GetOk("ip_address"); ok {
		p.SetIpaddress(ipaddress.(string))
	}

	// If there is a group supplied, add it to the parameter struct
	if group, ok := d.GetOk("group"); ok {
		p.SetGroup(group.(string))
	}

	// If there are affinity group IDs supplied, add them to the parameter struct
	if agIDs := d.Get("affinity_group_ids").(*schema.Set); agIDs.Len() > 0 {
		var groups []string
		for _, group := range agIDs.List() {
			groups = append(groups, group.(string))
		}
		p.SetAffinitygroupids(groups)
	}

	// If there are affinity group names supplied, add them to the parameter struct
	if agNames := d.Get("affinity_group_names").(*schema.Set); agNames.Len() > 0 {
		var groups []string
		for _, group := range agNames.List() {
			groups = append(groups, group.(string))
		}
		p.SetAffinitygroupnames(groups)
	}

	// If there are security group IDs supplied, add them to the parameter struct
	if sgIDs := d.Get("security_group_ids").(*schema.Set); sgIDs.Len() > 0 {
		var groups []string
		for _, group := range sgIDs.List() {
			groups = append(groups, group.(string))
		}
		p.SetSecuritygroupids(groups)
	}

	// If there are security group names supplied, add them to the parameter struct
	if sgNames := d.Get("security_group_names").(*schema.Set); sgNames.Len() > 0 {
		var groups []string
		for _, group := range sgNames.List() {
			groups = append(groups, group.(string))
		}
		p.SetSecuritygroupnames(groups)
	}

	// If there is a project supplied, we retrieve and set the project id
	if err := setProjectid(p, cs, d); err != nil {
		return err
	}

	// If a keypair is supplied, add it to the parameter struct
	if keypair, ok := d.GetOk("keypair"); ok {
		p.SetKeypair(keypair.(string))
	}

	if keypairs, ok := d.GetOk("keypairs"); ok {
		var keypairStrings []string
		for _, kp := range keypairs.([]interface{}) {
			keypairStrings = append(keypairStrings, fmt.Sprintf("%v", kp))
		}
		p.SetKeypairs(keypairStrings)
	}

	// If a host_id is supplied, add it to the parameter struct

	if hostid, ok := d.GetOk("host_id"); ok {
		p.SetHostid(hostid.(string))
	}

	// If a pod_id is supplied, add it to the parameter struct

	if podid, ok := d.GetOk("pod_id"); ok {
		p.SetPodid(podid.(string))
	}

	// If a cluster_id is supplied, add it to the parameter struct

	if clusterid, ok := d.GetOk("cluster_id"); ok {
		p.SetClusterid(clusterid.(string))
	}

	if userData, ok := d.GetOk("user_data"); ok {
		ud, err := getUserData(userData.(string))
		if err != nil {
			return err
		}
		p.SetUserdata(ud)
	}

	// Create the new instance
	r, err := cs.VirtualMachine.DeployVirtualMachine(p)
	if err != nil {
		return fmt.Errorf("Error creating the new instance %s: %s", name, err)
	}

	d.SetId(r.Id)

	// Set tags if necessary
	if err = setTags(cs, d, "userVm"); err != nil {
		return fmt.Errorf("Error setting tags on the new instance %s: %s", name, err)
	}

	// Set the connection info for any configured provisioners
	d.SetConnInfo(map[string]string{
		"host":     r.Nic[0].Ipaddress,
		"password": r.Password,
	})

	return resourceCloudStackInstanceRead(d, meta)
}

func resourceCloudStackInstanceRead(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the virtual machine details
	vm, count, err := cs.VirtualMachine.GetVirtualMachineByID(
		d.Id(),
		cloudstack.WithProject(d.Get("project").(string)),
	)
	if err != nil {
		if count == 0 {
			log.Printf("[DEBUG] Instance %s does no longer exist", d.Get("name").(string))
			d.SetId("")
			return nil
		}

		return err
	}

	// Update the config
	d.Set("name", vm.Name)
	d.Set("display_name", vm.Displayname)
	d.Set("group", vm.Group)

	// In some rare cases (when destroying a machine fails) it can happen that
	// an instance does not have any attached NIC anymore.
	if len(vm.Nic) > 0 {
		d.Set("network_id", vm.Nic[0].Networkid)
		d.Set("ip_address", vm.Nic[0].Ipaddress)
		if vm.Nic[0].Ip6address != "" {
			d.Set("ip6_address", vm.Nic[0].Ip6address)
			d.Set("ip6_cidr", vm.Nic[0].Ip6cidr)
		}
	}

	// Create a new param struct.
	p := cs.Volume.NewListVolumesParams()
	p.SetType("ROOT")
	p.SetVirtualmachineid(d.Id())

	// Get the root disk of the instance.
	l, err := cs.Volume.ListVolumes(p)
	if err != nil {
		return err
	}

	// If we found the root disk, then update its size.
	if len(l.Volumes) != 1 {
		log.Printf("[DEBUG] Failed to find root disk of instance: %s", vm.Name)
	} else {
		d.Set("root_disk_size", l.Volumes[0].Size>>30) // B to GiB
	}

	if _, ok := d.GetOk("affinity_group_ids"); ok {
		groups := &schema.Set{F: schema.HashString}
		for _, group := range vm.Affinitygroup {
			groups.Add(group.Id)
		}
		d.Set("affinity_group_ids", groups)
	}

	if _, ok := d.GetOk("affinity_group_names"); ok {
		groups := &schema.Set{F: schema.HashString}
		for _, group := range vm.Affinitygroup {
			groups.Add(group.Name)
		}
		d.Set("affinity_group_names", groups)
	}

	if _, ok := d.GetOk("security_group_ids"); ok {
		groups := &schema.Set{F: schema.HashString}
		for _, group := range vm.Securitygroup {
			groups.Add(group.Id)
		}
		d.Set("security_group_ids", groups)
	}

	if _, ok := d.GetOk("security_group_names"); ok {
		groups := &schema.Set{F: schema.HashString}
		for _, group := range vm.Securitygroup {
			groups.Add(group.Name)
		}
		d.Set("security_group_names", groups)
	}

	d.Set("tags", tagsToMap(vm.Tags))

	setValueOrID(d, "service_offering", vm.Serviceofferingname, vm.Serviceofferingid)
	setValueOrID(d, "template", vm.Templatename, vm.Templateid)
	setValueOrID(d, "project", vm.Project, vm.Projectid)
	setValueOrID(d, "zone", vm.Zonename, vm.Zoneid)

	return nil
}

func resourceCloudStackInstanceUpdate(d *schema.ResourceData, meta interface{}) error {

	cs := meta.(*cloudstack.CloudStackClient)

	name := d.Get("name").(string)

	// Check if the display name is changed and if so, update the virtual machine
	if d.HasChange("display_name") {
		log.Printf("[DEBUG] Display name changed for %s, starting update", name)

		// Create a new parameter struct
		p := cs.VirtualMachine.NewUpdateVirtualMachineParams(d.Id())

		// Set the new display name
		p.SetDisplayname(d.Get("display_name").(string))

		// Update the display name
		_, err := cs.VirtualMachine.UpdateVirtualMachine(p)
		if err != nil {
			return fmt.Errorf(
				"Error updating the display name for instance %s: %s", name, err)
		}

	}

	// Check if the group is changed and if so, update the virtual machine
	if d.HasChange("group") {
		log.Printf("[DEBUG] Group changed for %s, starting update", name)

		// Create a new parameter struct
		p := cs.VirtualMachine.NewUpdateVirtualMachineParams(d.Id())

		// Set the new group
		p.SetGroup(d.Get("group").(string))

		// Update the display name
		_, err := cs.VirtualMachine.UpdateVirtualMachine(p)
		if err != nil {
			return fmt.Errorf(
				"Error updating the group for instance %s: %s", name, err)
		}

	}

	// Attributes that require reboot to update
	if d.HasChange("name") || d.HasChange("service_offering") || d.HasChange("affinity_group_ids") ||
		d.HasChange("affinity_group_names") || d.HasChange("keypair") || d.HasChange("keypairs") || d.HasChange("user_data") {

		// Before we can actually make these changes, the virtual machine must be stopped
		_, err := cs.VirtualMachine.StopVirtualMachine(
			cs.VirtualMachine.NewStopVirtualMachineParams(d.Id()))
		if err != nil {
			return fmt.Errorf(
				"Error stopping instance %s before making changes: %s", name, err)
		}

		// Check if the name has changed and if so, update the name
		if d.HasChange("name") {
			log.Printf("[DEBUG] Name for %s changed to %s, starting update", d.Id(), name)

			// Create a new parameter struct
			p := cs.VirtualMachine.NewUpdateVirtualMachineParams(d.Id())

			// Set the new name
			p.SetName(name)

			// Update the display name
			_, err := cs.VirtualMachine.UpdateVirtualMachine(p)
			if err != nil {
				return fmt.Errorf(
					"Error updating the name for instance %s: %s", name, err)
			}

		}

		// Check if the service offering is changed and if so, update the offering
		if d.HasChange("service_offering") {
			log.Printf("[DEBUG] Service offering changed for %s, starting update", name)

			// Retrieve the service_offering ID
			serviceofferingid, e := retrieveID(cs, "service_offering", d.Get("service_offering").(string))
			if e != nil {
				return e.Error()
			}

			// Create a new parameter struct
			p := cs.VirtualMachine.NewChangeServiceForVirtualMachineParams(d.Id(), serviceofferingid)

			// Change the service offering
			_, err = cs.VirtualMachine.ChangeServiceForVirtualMachine(p)
			if err != nil {
				return fmt.Errorf(
					"Error changing the service offering for instance %s: %s", name, err)
			}
		}

		// Check if the affinity group IDs have changed and if so, update the IDs
		if d.HasChange("affinity_group_ids") {
			p := cs.AffinityGroup.NewUpdateVMAffinityGroupParams(d.Id())
			groups := []string{}

			if agIDs := d.Get("affinity_group_ids").(*schema.Set); agIDs.Len() > 0 {
				for _, group := range agIDs.List() {
					groups = append(groups, group.(string))
				}
			}

			// Set the new groups
			p.SetAffinitygroupids(groups)

			// Update the affinity groups
			_, err = cs.AffinityGroup.UpdateVMAffinityGroup(p)
			if err != nil {
				return fmt.Errorf(
					"Error updating the affinity groups for instance %s: %s", name, err)
			}
		}

		// Check if the affinity group names have changed and if so, update the names
		if d.HasChange("affinity_group_names") {
			p := cs.AffinityGroup.NewUpdateVMAffinityGroupParams(d.Id())
			groups := []string{}

			if agNames := d.Get("affinity_group_names").(*schema.Set); agNames.Len() > 0 {
				for _, group := range agNames.List() {
					groups = append(groups, group.(string))
				}
			}

			// Set the new groups
			p.SetAffinitygroupnames(groups)

			// Update the affinity groups
			_, err = cs.AffinityGroup.UpdateVMAffinityGroup(p)
			if err != nil {
				return fmt.Errorf(
					"Error updating the affinity groups for instance %s: %s", name, err)
			}
		}

		// Check if the keypair has changed and if so, update the keypair
		if d.HasChange("keypair") || d.HasChange("keypairs") {
			log.Printf("[DEBUG] SSH keypair(s) changed for %s, starting update", name)

			p := cs.SSH.NewResetSSHKeyForVirtualMachineParams(d.Id())

			if keypair, ok := d.GetOk("keypair"); ok {
				p.SetKeypair(keypair.(string))
			}

			if keypairs, ok := d.GetOk("keypairs"); ok {

				// Convert keypairsInterface to []interface{}
				keypairsInterfaces := keypairs.([]interface{})

				// Now, safely convert []interface{} to []string with error handling
				strKeyPairs := make([]string, len(keypairsInterfaces))

				for i, v := range keypairsInterfaces {
					switch v := v.(type) {
					case string:
						strKeyPairs[i] = v
					default:
						log.Printf("Value at index %d is not a string: %v", i, v)
						continue
					}
				}
				p.SetKeypairs(strKeyPairs)
			}

			// If there is a project supplied, we retrieve and set the project id
			if err := setProjectid(p, cs, d); err != nil {
				return err
			}
			// Change the ssh keypair
			_, err = cs.SSH.ResetSSHKeyForVirtualMachine(p)
			if err != nil {
				return fmt.Errorf(
					"Error changing the SSH keypair(s) for instance %s: %s", name, err)
			}
		}

		// Check if the user data has changed and if so, update the user data
		if d.HasChange("user_data") {
			log.Printf("[DEBUG] user_data changed for %s, starting update", name)

			ud, err := getUserData(d.Get("user_data").(string))
			if err != nil {
				return err
			}

			p := cs.VirtualMachine.NewUpdateVirtualMachineParams(d.Id())
			p.SetUserdata(ud)
			_, err = cs.VirtualMachine.UpdateVirtualMachine(p)
			if err != nil {
				return fmt.Errorf(
					"Error updating user_data for instance %s: %s", name, err)
			}
		}

		// Start the virtual machine again
		_, err = cs.VirtualMachine.StartVirtualMachine(
			cs.VirtualMachine.NewStartVirtualMachineParams(d.Id()))
		if err != nil {
			return fmt.Errorf(
				"Error starting instance %s after making changes", name)
		}
	}

	// Check if the tags have changed and if so, update the tags
	if d.HasChange("tags") {
		if err := updateTags(cs, d, "UserVm"); err != nil {
			return fmt.Errorf("Error updating tags on instance %s: %s", name, err)
		}
	}

	// Check if the details have changed and if so, update the details
	if d.HasChange("details") {
		p := cs.VirtualMachine.NewUpdateVirtualMachineParams(d.Id())
		vmDetails := make(map[string]string)
		if details := d.Get("details"); details != nil {
			for k, v := range details.(map[string]interface{}) {
				vmDetails[k] = v.(string)
			}
		}
		p.SetDetails(vmDetails)
	}

	return resourceCloudStackInstanceRead(d, meta)
}

func resourceCloudStackInstanceDelete(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.VirtualMachine.NewDestroyVirtualMachineParams(d.Id())

	if d.Get("expunge").(bool) {
		p.SetExpunge(true)
	}

	log.Printf("[INFO] Destroying instance: %s", d.Get("name").(string))
	if _, err := cs.VirtualMachine.DestroyVirtualMachine(p); err != nil {
		// This is a very poor way to be told the ID does no longer exist :(
		if strings.Contains(err.Error(), fmt.Sprintf(
			"Invalid parameter id value=%s due to incorrect long value format, "+
				"or entity does not exist", d.Id())) {
			return nil
		}

		return fmt.Errorf("Error destroying instance: %s", err)
	}

	return nil
}
func resourceCloudStackInstanceImportContext(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	// We set start_vm to true as that matches the default and we assume that
	// when you need to import an instance it means it is already running.
	d.Set("start_vm", true)
	return importStatePassthroughContext(ctx, d, meta)
}

// getUserData returns the user data as a base64 encoded string
func getUserData(userData string) (string, error) {
	ud := userData
	if _, err := base64.StdEncoding.DecodeString(ud); err != nil {
		ud = base64.StdEncoding.EncodeToString([]byte(userData))
	}

	return ud, nil
}
