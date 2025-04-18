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

func resourceCloudStackPrivateGateway() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudStackPrivateGatewayCreate,
		Read:   resourceCloudStackPrivateGatewayRead,
		Update: resourceCloudStackPrivateGatewayUpdate,
		Delete: resourceCloudStackPrivateGatewayDelete,
		Importer: &schema.ResourceImporter{
			StateContext: importStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"gateway": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"ip_address": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"netmask": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"vlan": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"physical_network_id": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"network_offering": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"acl_id": {
				Type:     schema.TypeString,
				Required: true,
			},

			"vpc_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"bypass_vlan_check": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
		},
	}
}

func resourceCloudStackPrivateGatewayCreate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	ipaddress := d.Get("ip_address").(string)
	networkofferingid := d.Get("network_offering").(string)

	// Create a new parameter struct
	p := cs.VPC.NewCreatePrivateGatewayParams(
		d.Get("gateway").(string),
		ipaddress,
		d.Get("netmask").(string),
		d.Get("vpc_id").(string),
	)
	p.SetVlan(d.Get("vlan").(string))
	p.SetBypassvlanoverlapcheck(d.Get("bypass_vlan_check").(bool))

	// Retrieve the network_offering ID
	if networkofferingid != "" {
		networkofferingid, e := retrieveID(cs, "network_offering", networkofferingid)
		if e != nil {
			return e.Error()
		}
		p.SetNetworkofferingid(networkofferingid)
	}

	// Check if we want to associate an ACL
	if aclid, ok := d.GetOk("acl_id"); ok {
		// Set the acl ID
		p.SetAclid(aclid.(string))
	}

	// Create the new private gateway
	r, err := cs.VPC.CreatePrivateGateway(p)
	if err != nil {
		return fmt.Errorf("Error creating private gateway for %s: %s", ipaddress, err)
	}

	d.SetId(r.Id)

	return resourceCloudStackPrivateGatewayRead(d, meta)
}

func resourceCloudStackPrivateGatewayRead(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the private gateway details
	gw, count, err := cs.VPC.GetPrivateGatewayByID(d.Id())
	if err != nil {
		if count == 0 {
			log.Printf("[DEBUG] Private gateway %s does no longer exist", d.Id())
			d.SetId("")
			return nil
		}

		return err
	}

	d.Set("gateway", gw.Gateway)
	d.Set("ip_address", gw.Ipaddress)
	d.Set("netmask", gw.Netmask)
	d.Set("vlan", strings.Replace(gw.Vlan, "vlan://", "", -1))
	d.Set("acl_id", gw.Aclid)
	d.Set("vpc_id", gw.Vpcid)

	return nil
}

func resourceCloudStackPrivateGatewayUpdate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Replace the ACL if the ID has changed
	if d.HasChange("acl_id") {
		p := cs.NetworkACL.NewReplaceNetworkACLListParams(d.Get("acl_id").(string))
		p.SetNetworkid(d.Id())

		_, err := cs.NetworkACL.ReplaceNetworkACLList(p)
		if err != nil {
			return fmt.Errorf("Error replacing ACL: %s", err)
		}
	}

	return resourceCloudStackNetworkRead(d, meta)
}

func resourceCloudStackPrivateGatewayDelete(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.VPC.NewDeletePrivateGatewayParams(d.Id())

	// Delete the private gateway
	_, err := cs.VPC.DeletePrivateGateway(p)
	if err != nil {
		// This is a very poor way to be told the ID does no longer exist :(
		if strings.Contains(err.Error(), fmt.Sprintf(
			"Invalid parameter id value=%s due to incorrect long value format, "+
				"or entity does not exist", d.Id())) {
			return nil
		}

		return fmt.Errorf("Error deleting private gateway %s: %s", d.Id(), err)
	}

	return nil
}
