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

func resourceCloudStackNetworkACL() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudStackNetworkACLCreate,
		Read:   resourceCloudStackNetworkACLRead,
		Delete: resourceCloudStackNetworkACLDelete,
		Importer: &schema.ResourceImporter{
			StateContext: importStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
				ForceNew: true,
			},

			"project": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
			},

			"vpc_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceCloudStackNetworkACLCreate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	name := d.Get("name").(string)

	// Create a new parameter struct
	p := cs.NetworkACL.NewCreateNetworkACLListParams(name, d.Get("vpc_id").(string))

	// Set the description
	if description, ok := d.GetOk("description"); ok {
		p.SetDescription(description.(string))
	} else {
		p.SetDescription(name)
	}

	// Create the new network ACL list
	r, err := cs.NetworkACL.CreateNetworkACLList(p)
	if err != nil {
		return fmt.Errorf("Error creating network ACL list %s: %s", name, err)
	}

	d.SetId(r.Id)

	return resourceCloudStackNetworkACLRead(d, meta)
}

func resourceCloudStackNetworkACLRead(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the network ACL list details
	f, count, err := cs.NetworkACL.GetNetworkACLListByID(
		d.Id(),
		cloudstack.WithProject(d.Get("project").(string)),
	)
	if err != nil {
		if count == 0 {
			log.Printf(
				"[DEBUG] Network ACL list %s does no longer exist", d.Get("name").(string))
			d.SetId("")
			return nil
		}

		return err
	}

	d.Set("name", f.Name)
	d.Set("description", f.Description)
	d.Set("vpc_id", f.Vpcid)

	return nil
}

func resourceCloudStackNetworkACLDelete(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.NetworkACL.NewDeleteNetworkACLListParams(d.Id())

	// Delete the network ACL list
	_, err := Retry(3, func() (interface{}, error) {
		return cs.NetworkACL.DeleteNetworkACLList(p)
	})
	if err != nil {
		// This is a very poor way to be told the ID does no longer exist :(
		if strings.Contains(err.Error(), fmt.Sprintf(
			"Invalid parameter id value=%s due to incorrect long value format, "+
				"or entity does not exist", d.Id())) {
			return nil
		}

		return fmt.Errorf("Error deleting network ACL list %s: %s", d.Get("name").(string), err)
	}

	return nil
}
