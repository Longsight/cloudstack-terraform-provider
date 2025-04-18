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
	"fmt"
	"strings"

	"github.com/apache/cloudstack-go/v2/cloudstack"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceCloudStackDisk() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudStackDiskCreate,
		Read:   resourceCloudStackDiskRead,
		Update: resourceCloudStackDiskUpdate,
		Delete: resourceCloudStackDiskDelete,
		Importer: &schema.ResourceImporter{
			StateContext: importStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"attach": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},

			"device_id": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},

			"disk_offering": {
				Type:     schema.TypeString,
				Optional: true,
			},

			"size": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},

			"shrink_ok": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"virtual_machine_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
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

			"tags": tagsSchema(),

			"reattach_on_change": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

func resourceCloudStackDiskCreate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	name := d.Get("name").(string)

	// Create a new parameter struct
	p := cs.Volume.NewCreateVolumeParams()
	p.SetName(name)

	// Retrieve the disk_offering ID
	diskofferingid, e := retrieveID(cs, "disk_offering", d.Get("disk_offering").(string))
	if e != nil {
		return e.Error()
	}
	// Set the disk_offering ID
	p.SetDiskofferingid(diskofferingid)

	if d.Get("size").(int) != 0 {
		// Set the volume size
		p.SetSize(int64(d.Get("size").(int)))
	}

	// If there is a project supplied, we retrieve and set the project id
	if err := setProjectid(p, cs, d); err != nil {
		return err
	}

	// Retrieve the zone ID
	zoneid, e := retrieveID(cs, "zone", d.Get("zone").(string))
	if e != nil {
		return e.Error()
	}
	// Set the zone ID
	p.SetZoneid(zoneid)

	// Create the new volume
	r, err := cs.Volume.CreateVolume(p)
	if err != nil {
		return fmt.Errorf("Error creating the new disk %s: %s", name, err)
	}

	// Set the volume ID and partials
	d.SetId(r.Id)

	// Set tags if necessary
	err = setTags(cs, d, "Volume")
	if err != nil {
		return fmt.Errorf("Error setting tags on the new disk %s: %s", name, err)
	}

	if d.Get("attach").(bool) {
		if err := resourceCloudStackDiskAttach(d, meta); err != nil {
			return fmt.Errorf("Error attaching the new disk %s to virtual machine: %s", name, err)
		}

		// Set the additional partial
	}

	return resourceCloudStackDiskRead(d, meta)
}

func resourceCloudStackDiskRead(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the volume details
	v, count, err := cs.Volume.GetVolumeByID(
		d.Id(),
		cloudstack.WithProject(d.Get("project").(string)),
	)
	if err != nil {
		if count == 0 {
			d.SetId("")
			return nil
		}

		return err
	}

	d.Set("name", v.Name)
	d.Set("attach", v.Virtualmachineid != "")   // If attached this contains a virtual machine ID
	d.Set("size", int(v.Size/(1024*1024*1024))) // Needed to get GB's again

	tags := make(map[string]interface{})
	for _, tag := range v.Tags {
		tags[tag.Key] = tag.Value
	}
	d.Set("tags", tags)

	setValueOrID(d, "disk_offering", v.Diskofferingname, v.Diskofferingid)
	setValueOrID(d, "project", v.Project, v.Projectid)
	setValueOrID(d, "zone", v.Zonename, v.Zoneid)

	if v.Virtualmachineid != "" {
		d.Set("device_id", int(v.Deviceid))
		d.Set("virtual_machine_id", v.Virtualmachineid)
	}

	return nil
}

func resourceCloudStackDiskUpdate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	name := d.Get("name").(string)

	if d.HasChange("disk_offering") || d.HasChange("size") {
		if d.Get("reattach_on_change").(bool) {
			// Detach the volume (re-attach is done at the end of this function)
			if err := resourceCloudStackDiskDetach(d, meta); err != nil {
				return fmt.Errorf("Error detaching disk %s from virtual machine: %s", name, err)
			}
		}

		// Create a new parameter struct
		p := cs.Volume.NewResizeVolumeParams(d.Id())

		// Retrieve the disk_offering ID
		diskofferingid, e := retrieveID(cs, "disk_offering", d.Get("disk_offering").(string))
		if e != nil {
			return e.Error()
		}

		// Set the disk_offering ID
		p.SetDiskofferingid(diskofferingid)

		if d.HasChange("size") {
			// Set the size
			p.SetSize(int64(d.Get("size").(int)))
		}

		// Set the shrink bit
		p.SetShrinkok(d.Get("shrink_ok").(bool))

		// Change the disk_offering
		r, err := cs.Volume.ResizeVolume(p)
		if err != nil {
			return fmt.Errorf("Error changing disk offering/size for disk %s: %s", name, err)
		}

		// Update the volume ID and set partials
		d.SetId(r.Id)
	}

	// If the device ID changed, just detach here so we can re-attach the
	// volume at the end of this function
	if d.HasChange("device_id") || d.HasChange("virtual_machine") {
		// Detach the volume
		if err := resourceCloudStackDiskDetach(d, meta); err != nil {
			return fmt.Errorf("Error detaching disk %s from virtual machine: %s", name, err)
		}
	}

	if d.Get("attach").(bool) {
		// Attach the volume
		err := resourceCloudStackDiskAttach(d, meta)
		if err != nil {
			return fmt.Errorf("Error attaching disk %s to virtual machine: %s", name, err)
		}

		// Set the additional partials
	} else {
		// Detach the volume
		if err := resourceCloudStackDiskDetach(d, meta); err != nil {
			return fmt.Errorf("Error detaching disk %s from virtual machine: %s", name, err)
		}
	}

	// Check is the tags have changed and if so, update the tags
	if d.HasChange("tags") {
		err := updateTags(cs, d, "Volume")
		if err != nil {
			return fmt.Errorf("Error updating tags on disk %s: %s", name, err)
		}
	}

	return resourceCloudStackDiskRead(d, meta)
}

func resourceCloudStackDiskDelete(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Detach the volume
	if err := resourceCloudStackDiskDetach(d, meta); err != nil {
		return err
	}

	// Create a new parameter struct
	p := cs.Volume.NewDeleteVolumeParams(d.Id())

	// Delete the voluem
	if _, err := cs.Volume.DeleteVolume(p); err != nil {
		// This is a very poor way to be told the ID does no longer exist :(
		if strings.Contains(err.Error(), fmt.Sprintf(
			"Invalid parameter id value=%s due to incorrect long value format, "+
				"or entity does not exist", d.Id())) {
			return nil
		}

		return err
	}

	return nil
}

func resourceCloudStackDiskAttach(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	if virtualmachineid, ok := d.GetOk("virtual_machine_id"); ok {
		// First check if the disk isn't already attached
		if attached, err := isAttached(d, meta); err != nil || attached {
			return err
		}

		// Create a new parameter struct
		p := cs.Volume.NewAttachVolumeParams(d.Id(), virtualmachineid.(string))

		if deviceid, ok := d.GetOk("device_id"); ok {
			p.SetDeviceid(int64(deviceid.(int)))
		}

		// Attach the new volume
		r, err := Retry(10, retryableAttachVolumeFunc(cs, p))
		if err != nil {
			return fmt.Errorf("Error attaching volume to VM: %s", err)
		}

		d.SetId(r.(*cloudstack.AttachVolumeResponse).Id)
	}

	return nil
}

func resourceCloudStackDiskDetach(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Check if the volume is actually attached, before detaching
	if attached, err := isAttached(d, meta); err != nil || !attached {
		return err
	}

	// Create a new parameter struct
	p := cs.Volume.NewDetachVolumeParams()

	// Set the volume ID
	p.SetId(d.Id())

	// Detach the currently attached volume
	_, err := cs.Volume.DetachVolume(p)
	if err != nil {
		if virtualmachineid, ok := d.GetOk("virtual_machine_id"); ok {
			// Create a new parameter struct
			pd := cs.VirtualMachine.NewStopVirtualMachineParams(virtualmachineid.(string))

			// Stop the virtual machine in order to be able to detach the disk
			if _, err := cs.VirtualMachine.StopVirtualMachine(pd); err != nil {
				return err
			}

			// Try again to detach the currently attached volume
			if _, err := cs.Volume.DetachVolume(p); err != nil {
				return err
			}

			// Create a new parameter struct
			pu := cs.VirtualMachine.NewStartVirtualMachineParams(virtualmachineid.(string))

			// Start the virtual machine again
			if _, err := cs.VirtualMachine.StartVirtualMachine(pu); err != nil {
				return err
			}
		}
	}

	return err
}

func isAttached(d *schema.ResourceData, meta interface{}) (bool, error) {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the volume details
	v, _, err := cs.Volume.GetVolumeByID(
		d.Id(),
		cloudstack.WithProject(d.Get("project").(string)),
	)
	if err != nil {
		return false, err
	}

	return v.Virtualmachineid != "", nil
}

func retryableAttachVolumeFunc(
	cs *cloudstack.CloudStackClient,
	p *cloudstack.AttachVolumeParams) func() (interface{}, error) {
	return func() (interface{}, error) {
		r, err := cs.Volume.AttachVolume(p)
		if err != nil {
			return nil, err
		}
		return r, nil
	}
}
