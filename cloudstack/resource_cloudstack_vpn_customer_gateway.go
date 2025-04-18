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

func resourceCloudStackVPNCustomerGateway() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudStackVPNCustomerGatewayCreate,
		Read:   resourceCloudStackVPNCustomerGatewayRead,
		Update: resourceCloudStackVPNCustomerGatewayUpdate,
		Delete: resourceCloudStackVPNCustomerGatewayDelete,
		Importer: &schema.ResourceImporter{
			StateContext: importStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},

			"cidr": {
				Type:     schema.TypeString,
				Required: true,
			},

			"esp_policy": {
				Type:     schema.TypeString,
				Required: true,
			},

			"gateway": {
				Type:     schema.TypeString,
				Required: true,
			},

			"ike_policy": {
				Type:     schema.TypeString,
				Required: true,
			},

			"ipsec_psk": {
				Type:     schema.TypeString,
				Required: true,
			},

			"dpd": {
				Type:     schema.TypeBool,
				Optional: true,
				Computed: true,
			},

			"esp_lifetime": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},

			"ike_lifetime": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},

			"project": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},
		},
	}
}

func resourceCloudStackVPNCustomerGatewayCreate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.VPN.NewCreateVpnCustomerGatewayParams(
		d.Get("cidr").(string),
		d.Get("esp_policy").(string),
		d.Get("gateway").(string),
		d.Get("ike_policy").(string),
		d.Get("ipsec_psk").(string),
	)

	p.SetName(d.Get("name").(string))

	if dpd, ok := d.GetOk("dpd"); ok {
		p.SetDpd(dpd.(bool))
	}

	if esplifetime, ok := d.GetOk("esp_lifetime"); ok {
		p.SetEsplifetime(int64(esplifetime.(int)))
	}

	if ikelifetime, ok := d.GetOk("ike_lifetime"); ok {
		p.SetIkelifetime(int64(ikelifetime.(int)))
	}

	// If there is a project supplied, we retrieve and set the project id
	if err := setProjectid(p, cs, d); err != nil {
		return err
	}

	// Create the new VPN Customer Gateway
	v, err := cs.VPN.CreateVpnCustomerGateway(p)
	if err != nil {
		return fmt.Errorf("Error creating VPN Customer Gateway %s: %s", d.Get("name").(string), err)
	}

	d.SetId(v.Id)

	return resourceCloudStackVPNCustomerGatewayRead(d, meta)
}

func resourceCloudStackVPNCustomerGatewayRead(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the VPN Customer Gateway details
	v, count, err := cs.VPN.GetVpnCustomerGatewayByID(d.Id())
	if err != nil {
		if count == 0 {
			log.Printf(
				"[DEBUG] VPN Customer Gateway %s does no longer exist", d.Get("name").(string))
			d.SetId("")
			return nil
		}

		return err
	}

	d.Set("name", v.Name)
	d.Set("cidr", v.Cidrlist)
	d.Set("esp_policy", v.Esppolicy)
	d.Set("gateway", v.Gateway)
	d.Set("ike_policy", v.Ikepolicy)
	d.Set("ipsec_psk", v.Ipsecpsk)
	d.Set("dpd", v.Dpd)
	d.Set("esp_lifetime", int(v.Esplifetime))
	d.Set("ike_lifetime", int(v.Ikelifetime))

	setValueOrID(d, "project", v.Project, v.Projectid)

	return nil
}

func resourceCloudStackVPNCustomerGatewayUpdate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.VPN.NewUpdateVpnCustomerGatewayParams(
		d.Get("cidr").(string),
		d.Get("esp_policy").(string),
		d.Get("gateway").(string),
		d.Id(),
		d.Get("ike_policy").(string),
		d.Get("ipsec_psk").(string),
	)

	p.SetName(d.Get("name").(string))

	if dpd, ok := d.GetOk("dpd"); ok {
		p.SetDpd(dpd.(bool))
	}

	if esplifetime, ok := d.GetOk("esp_lifetime"); ok {
		p.SetEsplifetime(int64(esplifetime.(int)))
	}

	if ikelifetime, ok := d.GetOk("ike_lifetime"); ok {
		p.SetIkelifetime(int64(ikelifetime.(int)))
	}

	// Update the VPN Customer Gateway
	_, err := cs.VPN.UpdateVpnCustomerGateway(p)
	if err != nil {
		return fmt.Errorf("Error updating VPN Customer Gateway %s: %s", d.Get("name").(string), err)
	}

	return resourceCloudStackVPNCustomerGatewayRead(d, meta)
}

func resourceCloudStackVPNCustomerGatewayDelete(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.VPN.NewDeleteVpnCustomerGatewayParams(d.Id())

	// Delete the VPN Customer Gateway
	_, err := cs.VPN.DeleteVpnCustomerGateway(p)
	if err != nil {
		// This is a very poor way to be told the ID does no longer exist :(
		if strings.Contains(err.Error(), fmt.Sprintf(
			"Invalid parameter id value=%s due to incorrect long value format, "+
				"or entity does not exist", d.Id())) {
			return nil
		}

		return fmt.Errorf("Error deleting VPN Customer Gateway %s: %s", d.Get("name").(string), err)
	}

	return nil
}
