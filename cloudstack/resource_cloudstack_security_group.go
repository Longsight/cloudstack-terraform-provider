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

func resourceCloudStackSecurityGroup() *schema.Resource {
	return &schema.Resource{
		Create: resourceCloudStackSecurityGroupCreate,
		Read:   resourceCloudStackSecurityGroupRead,
		Delete: resourceCloudStackSecurityGroupDelete,
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
				Computed: true,
				ForceNew: true,
			},
		},
	}
}

func resourceCloudStackSecurityGroupCreate(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	name := d.Get("name").(string)

	// Create a new parameter struct
	p := cs.SecurityGroup.NewCreateSecurityGroupParams(name)

	// Set the description
	if description, ok := d.GetOk("description"); ok {
		p.SetDescription(description.(string))
	} else {
		p.SetDescription(name)
	}

	// If there is a project supplied, we retrieve and set the project id
	if err := setProjectid(p, cs, d); err != nil {
		return err
	}

	r, err := cs.SecurityGroup.CreateSecurityGroup(p)
	if err != nil {
		return fmt.Errorf("Error creating security group %s: %s", name, err)
	}

	d.SetId(r.Id)

	return resourceCloudStackSecurityGroupRead(d, meta)
}

func resourceCloudStackSecurityGroupRead(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Get the security group details
	sg, count, err := cs.SecurityGroup.GetSecurityGroupByID(
		d.Id(),
		cloudstack.WithProject(d.Get("project").(string)),
	)
	if err != nil {
		if count == 0 {
			log.Printf("[DEBUG] Security group %s does not longer exist", d.Get("name").(string))
			d.SetId("")
			return nil
		}

		return err
	}

	// Update the config
	d.Set("name", sg.Name)
	d.Set("description", sg.Description)

	setValueOrID(d, "project", sg.Project, sg.Projectid)

	return nil
}

func resourceCloudStackSecurityGroupDelete(d *schema.ResourceData, meta interface{}) error {
	cs := meta.(*cloudstack.CloudStackClient)

	// Create a new parameter struct
	p := cs.SecurityGroup.NewDeleteSecurityGroupParams()
	p.SetId(d.Id())

	// If there is a project supplied, we retrieve and set the project id
	if err := setProjectid(p, cs, d); err != nil {
		return err
	}

	// Delete the security group
	_, err := cs.SecurityGroup.DeleteSecurityGroup(p)
	if err != nil {
		// This is a very poor way to be told the ID does no longer exist :(
		if strings.Contains(err.Error(), fmt.Sprintf(
			"Invalid parameter id value=%s due to incorrect long value format, "+
				"or entity does not exist", d.Id())) {
			return nil
		}

		return fmt.Errorf("Error deleting security group: %s", err)
	}

	return nil
}
