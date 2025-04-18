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
	"log"
	"strings"

	"github.com/apache/cloudstack-go/v2/cloudstack"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceCloudStackVPC() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudStackVPCCreate,
		Read:   resourceCloudStackVPCRead,
		Update: resourceCloudStackVPCUpdate,
		Delete: resourceCloudStackVPCDelete,
		Importer: &schema.ResourceImporter{
			StateContext: importStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},

			"display_text": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"cidr": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"vpc_offering": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"network_domain": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"project": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"source_nat_ip": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"zone": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"tags": tagsSchema(),
		},
	}
}

func resourceCloudStackVPCCreate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	name := d.Get("name").(string)

	// Retrieve the vpc_offering ID
	vpcofferingid, e := retrieveID(cs, "vpc_offering", d.Get("vpc_offering").(string))
	if e != nil {
		return e.Error()
	}

	// Retrieve the zone ID
	zoneid, e := retrieveID(cs, "zone", d.Get("zone").(string))
	if e != nil {
		return e.Error()
	}

	// Set the display text
	displaytext, ok := d.GetOk("display_text")
	if !ok {
		displaytext = name
	}

	// Create a new parameter struct
	p := cs.VPC.NewCreateVPCParams(
		displaytext.(string),
		name,
		vpcofferingid,
		zoneid,
	)
	p.SetCidr(d.Get("cidr").(string))

	// If there is a network domain supplied, make sure to add it to the request
	if networkDomain, ok := d.GetOk("network_domain"); ok {
		// Set the network domain
		p.SetNetworkdomain(networkDomain.(string))
	}

	// If there is a project supplied, we retrieve and set the project id
	if err := setProjectid(p, cs, d); err != nil {
		return err
	}

	// Create the new VPC
	r, err := cs.VPC.CreateVPC(p)
	if err != nil {
		return fmt.Errorf("Error creating VPC %s: %s", name, err)
	}

	d.SetId(r.Id)

	// Set tags if necessary
	err = setTags(cs, d, "Vpc")
	if err != nil {
		return fmt.Errorf("Error setting tags on the VPC: %s", err)
	}

	return resourceCloudStackVPCRead(d, meta)
}

func resourceCloudStackVPCRead(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the VPC details
	v, count, err := cs.VPC.GetVPCByID(
		d.Id(),
		cloudstack.WithProject(d.Get("project").(string)),
	)
	if err != nil {
		if count == 0 {
			log.Printf(
				"[DEBUG] VPC %s does no longer exist", d.Get("name").(string))
			d.SetId("")
			return nil
		}

		return err
	}

	d.Set("name", v.Name)
	d.Set("display_text", v.Displaytext)
	d.Set("cidr", v.Cidr)
	d.Set("network_domain", v.Networkdomain)

	tags := make(map[string]interface{})
	for _, tag := range v.Tags {
		tags[tag.Key] = tag.Value
	}
	d.Set("tags", tags)

	// Get the VPC offering details
	o, _, err := cs.VPC.GetVPCOfferingByID(v.Vpcofferingid)
	if err != nil {
		return err
	}

	setValueOrID(d, "vpc_offering", o.Name, v.Vpcofferingid)
	setValueOrID(d, "project", v.Project, v.Projectid)
	setValueOrID(d, "zone", v.Zonename, v.Zoneid)

	// Create a new parameter struct
	p := cs.Address.NewListPublicIpAddressesParams()
	p.SetVpcid(d.Id())
	p.SetIssourcenat(true)

	// If there is a project supplied, we retrieve and set the project id
	if err := setProjectid(p, cs, d); err != nil {
		return err
	}

	// Get the source NAT IP assigned to the VPC
	l, err := cs.Address.ListPublicIpAddresses(p)
	if err != nil {
		return err
	}

	if l.Count == 1 {
		d.Set("source_nat_ip", l.PublicIpAddresses[0].Ipaddress)
	}

	return nil
}

func resourceCloudStackVPCUpdate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	name := d.Get("name").(string)

	// Check if the name is changed
	if d.HasChange("name") {
		// Create a new parameter struct
		p := cs.VPC.NewUpdateVPCParams(d.Id())

		// Set the new name
		p.SetName(name)

		// Update the VPC
		_, err := cs.VPC.UpdateVPC(p)
		if err != nil {
			return fmt.Errorf(
				"Error updating name of VPC %s: %s", name, err)
		}
	}

	// Check if the display text is changed
	if d.HasChange("display_text") {
		// Create a new parameter struct
		p := cs.VPC.NewUpdateVPCParams(d.Id())

		// Set the display text
		displaytext, ok := d.GetOk("display_text")
		if !ok {
			displaytext = d.Get("name")
		}

		// Set the new display text
		p.SetDisplaytext(displaytext.(string))

		// Update the VPC
		_, err := cs.VPC.UpdateVPC(p)
		if err != nil {
			return fmt.Errorf(
				"Error updating display test of VPC %s: %s", name, err)
		}
	}

	// Check is the tags have changed
	if d.HasChange("tags") {
		err := updateTags(cs, d, "Vpc")
		if err != nil {
			return fmt.Errorf("Error updating tags on VPC %s: %s", name, err)
		}
	}

	return resourceCloudStackVPCRead(d, meta)
}

func resourceCloudStackVPCDelete(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.VPC.NewDeleteVPCParams(d.Id())

	// Delete the VPC
	_, err := cs.VPC.DeleteVPC(p)
	if err != nil {
		// This is a very poor way to be told the ID does no longer exist :(
		if strings.Contains(err.Error(), fmt.Sprintf(
			"Invalid parameter id value=%s due to incorrect long value format, "+
				"or entity does not exist", d.Id())) {
			return nil
		}

		return fmt.Errorf("Error deleting VPC %s: %s", d.Get("name").(string), err)
	}

	return nil
}
