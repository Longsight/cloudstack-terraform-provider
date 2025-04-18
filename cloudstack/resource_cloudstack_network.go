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
	"net"
	"strconv"
	"strings"

	"github.com/apache/cloudstack-go/v2/cloudstack"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const none = "none"

func resourceCloudStackNetwork() *schema.Resource {
	aclidSchema := &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
		Default:  none,
	}

	aclidSchema.StateFunc = func(v interface{}) string {
		value := v.(string)

		if value == none {
			aclidSchema.ForceNew = true
		} else {
			aclidSchema.ForceNew = false
		}

		return value
	}

	return &schema.Resource{
		Create: resourceCloudStackNetworkCreate,
		Read:   resourceCloudStackNetworkRead,
		Update: resourceCloudStackNetworkUpdate,
		Delete: resourceCloudStackNetworkDelete,
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

			"gateway": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"startip": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"endip": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"network_domain": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},

			"network_offering": {
				Type:     schema.TypeString,
				Required: true,
			},

			"vlan": {
				Type:     schema.TypeInt,
				Optional: true,
				ForceNew: true,
			},

			"vpc_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"acl_id": aclidSchema,

			"project": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"source_nat_ip": {
				Type:     schema.TypeBool,
				Optional: true,
				ForceNew: true,
			},

			"source_nat_ip_address": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"source_nat_ip_id": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"zone": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"bypass_vlan_check": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},

			"tags": tagsSchema(),
		},
	}
}

func resourceCloudStackNetworkCreate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	name := d.Get("name").(string)

	// Retrieve the network_offering ID
	networkofferingid, e := retrieveID(cs, "network_offering", d.Get("network_offering").(string))
	if e != nil {
		return e.Error()
	}

	// Retrieve the zone ID
	zoneid, e := retrieveID(cs, "zone", d.Get("zone").(string))
	if e != nil {
		return e.Error()
	}

	// Create a new parameter struct
	p := cs.Network.NewCreateNetworkParams(name, networkofferingid, zoneid)

	if displaytext, ok := d.GetOk("display_text"); ok {
		p.SetDisplaytext(displaytext.(string))
	} else {
		p.SetDisplaytext(name)
	}

	// Get the network offering to check if it supports specifying IP ranges
	no, _, err := cs.NetworkOffering.GetNetworkOfferingByID(networkofferingid)
	if err != nil {
		return err
	}

	m, err := parseCIDR(d, no.Specifyipranges)
	if err != nil {
		return err
	}

	// Set the needed IP config
	p.SetGateway(m["gateway"])
	p.SetNetmask(m["netmask"])

	// Only set the start IP if we have one
	if startip, ok := m["startip"]; ok {
		p.SetStartip(startip)
	}

	// Only set the end IP if we have one
	if endip, ok := m["endip"]; ok {
		p.SetEndip(endip)
	}

	// Set the network domain if we have one
	if networkDomain, ok := d.GetOk("network_domain"); ok {
		p.SetNetworkdomain(networkDomain.(string))
	}

	if vlan, ok := d.GetOk("vlan"); ok {
		p.SetVlan(strconv.Itoa(vlan.(int)))
	}

	// Bypass VLAN overlap check if necessary
	p.SetBypassvlanoverlapcheck(d.Get("bypass_vlan_check").(bool))

	// Check is this network needs to be created in a VPC
	if vpcid, ok := d.GetOk("vpc_id"); ok {
		// Set the vpc id
		p.SetVpcid(vpcid.(string))

		// Since we're in a VPC, check if we want to associate an ACL list
		if aclid, ok := d.GetOk("acl_id"); ok && aclid.(string) != none {
			// Set the acl ID
			p.SetAclid(aclid.(string))
		}
	}

	// If there is a project supplied, we retrieve and set the project id
	if err := setProjectid(p, cs, d); err != nil {
		return err
	}

	// Create the new network
	r, err := cs.Network.CreateNetwork(p)
	if err != nil {
		return fmt.Errorf("Error creating network %s: %s", name, err)
	}

	d.SetId(r.Id)

	// Set tags if necessary
	if err = setTags(cs, d, "network"); err != nil {
		return fmt.Errorf("Error setting tags: %v", err)
	}

	if d.Get("source_nat_ip").(bool) {
		// Create a new parameter struct
		p := cs.Address.NewAssociateIpAddressParams()

		// Set required options
		p.SetNetworkid(r.Id)
		p.SetZoneid(zoneid)

		if vpcid, ok := d.GetOk("vpc_id"); ok {
			// Set the vpcid
			p.SetVpcid(vpcid.(string))
		}

		// If there is a project supplied, we retrieve and set the project id
		if err := setProjectid(p, cs, d); err != nil {
			return err
		}

		// Associate a new IP address
		ip, err := cs.Address.AssociateIpAddress(p)
		if err != nil {
			return fmt.Errorf("Error associating a new IP address: %s", err)
		}
		d.Set("source_nat_ip_address", ip.Ipaddress)
		d.Set("source_nat_ip_id", ip.Id)

		// Set the additional partial
	}

	return resourceCloudStackNetworkRead(d, meta)
}

func resourceCloudStackNetworkRead(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the virtual machine details
	n, count, err := cs.Network.GetNetworkByID(
		d.Id(),
		cloudstack.WithProject(d.Get("project").(string)),
	)
	if err != nil {
		if count == 0 {
			log.Printf(
				"[DEBUG] Network %s does no longer exist", d.Get("name").(string))
			d.SetId("")
			return nil
		}

		return err
	}

	d.Set("name", n.Name)
	d.Set("display_text", n.Displaytext)
	d.Set("cidr", n.Cidr)
	d.Set("gateway", n.Gateway)
	d.Set("network_domain", n.Networkdomain)
	d.Set("vpc_id", n.Vpcid)

	if n.Aclid == "" {
		n.Aclid = none
	}
	d.Set("acl_id", n.Aclid)

	tags := make(map[string]interface{})
	for _, tag := range n.Tags {
		tags[tag.Key] = tag.Value
	}
	d.Set("tags", tags)

	setValueOrID(d, "network_offering", n.Networkofferingname, n.Networkofferingid)
	setValueOrID(d, "project", n.Project, n.Projectid)
	setValueOrID(d, "zone", n.Zonename, n.Zoneid)

	if d.Get("source_nat_ip").(bool) {
		ip, count, err := cs.Address.GetPublicIpAddressByID(
			d.Get("source_nat_ip_id").(string),
			cloudstack.WithProject(d.Get("project").(string)),
		)
		if err != nil {
			if count == 0 {
				log.Printf(
					"[DEBUG] Source NAT IP with ID %s is no longer associated", d.Id())
				d.Set("source_nat_ip", false)
				d.Set("source_nat_ip_id", "")
				return nil
			}

			return err
		}

		if n.Id != ip.Associatednetworkid {
			d.Set("source_nat_ip", false)
			d.Set("source_nat_ip_id", "")
		}
	}

	return nil
}

func resourceCloudStackNetworkUpdate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)
	name := d.Get("name").(string)

	// Create a new parameter struct
	p := cs.Network.NewUpdateNetworkParams(d.Id())

	// Check if the name or display text is changed
	if d.HasChange("name") || d.HasChange("display_text") {
		p.SetName(name)

		// Compute/set the display text
		displaytext := d.Get("display_text").(string)
		if displaytext == "" {
			displaytext = name
		}
		p.SetDisplaytext(displaytext)
	}

	// Check if the cidr is changed
	if d.HasChange("cidr") {
		p.SetGuestvmcidr(d.Get("cidr").(string))
	}

	// Check if the network domain is changed
	if d.HasChange("network_domain") {
		p.SetNetworkdomain(d.Get("network_domain").(string))
	}

	// Check if the network offering is changed
	if d.HasChange("network_offering") {
		// Retrieve the network_offering ID
		networkofferingid, e := retrieveID(cs, "network_offering", d.Get("network_offering").(string))
		if e != nil {
			return e.Error()
		}
		// Set the new network offering
		p.SetNetworkofferingid(networkofferingid)
	}

	// Update the network
	_, err := cs.Network.UpdateNetwork(p)
	if err != nil {
		return fmt.Errorf(
			"Error updating network %s: %s", name, err)
	}

	// Replace the ACL if the ID has changed
	if d.HasChange("acl_id") {
		p := cs.NetworkACL.NewReplaceNetworkACLListParams(d.Get("acl_id").(string))
		p.SetNetworkid(d.Id())

		_, err := cs.NetworkACL.ReplaceNetworkACLList(p)
		if err != nil {
			return fmt.Errorf("Error replacing ACL: %s", err)
		}
	}

	// Update tags if they have changed
	if d.HasChange("tags") {
		if err := updateTags(cs, d, "Network"); err != nil {
			return fmt.Errorf("Error updating tags on ACL %s: %s", name, err)
		}
	}

	return resourceCloudStackNetworkRead(d, meta)
}

func resourceCloudStackNetworkDelete(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.Network.NewDeleteNetworkParams(d.Id())

	// Delete the network
	_, err := cs.Network.DeleteNetwork(p)
	if err != nil {
		// This is a very poor way to be told the ID does no longer exist :(
		if strings.Contains(err.Error(), fmt.Sprintf(
			"Invalid parameter id value=%s due to incorrect long value format, "+
				"or entity does not exist", d.Id())) {
			return nil
		}

		return fmt.Errorf("Error deleting network %s: %s", d.Get("name").(string), err)
	}
	return nil
}

func parseCIDR(d *schema.ResourceData, specifyiprange bool) (map[string]string, error) {
	m := make(map[string]string, 4)

	cidr := d.Get("cidr").(string)
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse cidr %s: %s", cidr, err)
	}

	msk := ipnet.Mask
	sub := ip.Mask(msk)

	m["netmask"] = fmt.Sprintf("%d.%d.%d.%d", msk[0], msk[1], msk[2], msk[3])

	if gateway, ok := d.GetOk("gateway"); ok {
		m["gateway"] = gateway.(string)
	} else {
		m["gateway"] = fmt.Sprintf("%d.%d.%d.%d", sub[0], sub[1], sub[2], sub[3]+1)
	}

	if startip, ok := d.GetOk("startip"); ok {
		m["startip"] = startip.(string)
	} else if specifyiprange {
		m["startip"] = fmt.Sprintf("%d.%d.%d.%d", sub[0], sub[1], sub[2], sub[3]+2)
	}

	if endip, ok := d.GetOk("endip"); ok {
		m["endip"] = endip.(string)
	} else if specifyiprange {
		m["endip"] = fmt.Sprintf("%d.%d.%d.%d",
			sub[0]+(0xff-msk[0]), sub[1]+(0xff-msk[1]), sub[2]+(0xff-msk[2]), sub[3]+(0xff-msk[3]-1))
	}

	return m, nil
}
