package exchange

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/open-horizon/anax/businesspolicy"
	"github.com/open-horizon/anax/cli/cliconfig"
	"github.com/open-horizon/anax/cli/cliutils"
	"github.com/open-horizon/anax/exchange"
	"github.com/open-horizon/anax/externalpolicy"
	"github.com/open-horizon/anax/policy"
	"net/http"
)

const BUSINESS_POLICY_TEMPLATE_OBJECT = `{
  "label": "",            /* Business policy label. */
  "description": "",      /* Business policy description. */
  "service": {
    "name": "",           /* The name of the service. */
    "org": "",            /* The org of the service. */
    "arch": "",           /* Set to '*' to use services of any hardware architecture. */
    "serviceVersions": [  /* A list of service versions. */
      {
        "version": "",
        "priority":{}
      }
    ]
  },
  "properties": [         /* A list of policy properties that describe the service being dployed. */
    {
      "name": "",
      "value": nil
    }
  ],
  "constraints": [        /* A list of constraint expressions of the form <property name> <operator> <property value>, */
                          /* separated by boolean operators AND (&&) or OR (||). */
    ""
  ],
  "userInput": [          /* A list of userInput variables to set when the service runs, listed by service. */
    {
      "serviceOrgid": "",         /* The org of the service. */
      "serviceUrl": "",           /* The name of the service. */
      "serviceVersionRange": "",  /* The service version range to which these variables should be applied. */
      "inputs": [                 /* The input variables to be set. */
        {
          "name": "",
          "value": nil
        }
      ]
    }
  ]
}`

//BusinessListPolicy lists all the policies in the org or only the specified policy if one is given
func BusinessListPolicy(org string, credToUse string, policy string, namesOnly bool) {
	cliutils.SetWhetherUsingApiKey(credToUse)
	var credOrg string
	credOrg, credToUse = cliutils.TrimOrg(org, credToUse)

	var polOrg string
	polOrg, policy = cliutils.TrimOrg(credOrg, policy)

	if policy == "*" {
		policy = ""
	}
	//get policy list from Horizon Exchange
	var policyList exchange.GetBusinessPolicyResponse
	httpCode := cliutils.ExchangeGet("Exchange", cliutils.GetExchangeUrl(), "orgs/"+polOrg+"/business/policies"+cliutils.AddSlash(policy), cliutils.OrgAndCreds(org, credToUse), []int{200, 404}, &policyList)
	if httpCode == 404 && policy != "" {
		cliutils.Fatal(cliutils.NOT_FOUND, "Policy %s not found in org %s", policy, polOrg)
	} else if httpCode == 404 {
		cliutils.Fatal(cliutils.NOT_FOUND, "Business policy for organization %s not found", polOrg)
	}

	if namesOnly && (policy == "" || policy == "*") {
		policyNameList := []string{}
		for bPolicy := range policyList.BusinessPolicy {
			policyNameList = append(policyNameList, bPolicy)
		}
		jsonBytes, err := json.MarshalIndent(policyNameList, "", cliutils.JSON_INDENT)
		if err != nil {
			cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to marshal 'hzn exchange business listpolicy' output: %v", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		buf := new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", cliutils.JSON_INDENT)
		err := enc.Encode(policyList.BusinessPolicy)
		if err != nil {
			cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to marshal 'hzn exchange business listpolicy' output: %v", err)
		}
		fmt.Println(string(buf.String()))
	}
}

//BusinessAddPolicy will add a new policy or overwrite an existing policy byt he same name in the Horizon Exchange
func BusinessAddPolicy(org string, credToUse string, policy string, jsonFilePath string) {
	cliutils.SetWhetherUsingApiKey(credToUse)
	org, credToUse = cliutils.TrimOrg(org, credToUse)
	org, policy = cliutils.TrimOrg(org, policy)

	//read in the new business policy from file
	newBytes := cliconfig.ReadJsonFileWithLocalConfig(jsonFilePath)
	var policyFile businesspolicy.BusinessPolicy
	err := json.Unmarshal(newBytes, &policyFile)
	if err != nil {
		cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to unmarshal json input file %s: %v", jsonFilePath, err)
	}

	//validate the format of the business policy
	err = policyFile.Validate()
	if err != nil {
		cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "Incorrect business policy format in file %s: %v", jsonFilePath, err)
	}

	//add/overwrite business policy file
	httpCode := cliutils.ExchangePutPost("Exchange", http.MethodPost, cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policy), cliutils.OrgAndCreds(org, credToUse), []int{201, 403}, policyFile)
	if httpCode == 403 {
		cliutils.ExchangePutPost("Exchange", http.MethodPut, cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policy), cliutils.OrgAndCreds(org, credToUse), []int{201, 404}, policyFile)
		fmt.Println("Business policy: " + org + "/" + policy + " updated in the Horizon Exchange")
	} else {
		fmt.Println("Business policy: " + org + "/" + policy + " added in the Horizon Exchange")
	}
}

//BusinessUpdatePolicy will replace a single attribute of a business policy in the Horizon Exchange
func BusinessUpdatePolicy(org string, credToUse string, policyName string, filePath string) {
	cliutils.SetWhetherUsingApiKey(credToUse)
	org, credToUse = cliutils.TrimOrg(org, credToUse)
	org, policyName = cliutils.TrimOrg(org, policyName)

	//Read in the file
	attribute := cliconfig.ReadJsonFileWithLocalConfig(filePath)

	//verify that the policy exists
	var exchangePolicy exchange.GetBusinessPolicyResponse
	httpCode := cliutils.ExchangeGet("Exchange", cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policyName), cliutils.OrgAndCreds(org, credToUse), []int{200, 404}, &exchangePolicy)
	if httpCode == 404 {
		cliutils.Fatal(cliutils.NOT_FOUND, "Policy %s not found in org %s", policyName, org)
	}

	findPatchType := make(map[string]interface{})

	json.Unmarshal([]byte(attribute), &findPatchType)

	if _, ok := findPatchType["service"]; ok {
		patch := make(map[string]businesspolicy.ServiceRef)
		err := json.Unmarshal([]byte(attribute), &patch)
		if err != nil {
			cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to unmarshal attribute input %s: %v", attribute, err)
		}
		cliutils.ExchangePutPost("Exchange", http.MethodPatch, cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policyName), cliutils.OrgAndCreds(org, credToUse), []int{201}, patch)
		fmt.Println("Policy " + org + "/" + policyName + " updated in the Horizon Exchange")
	} else if _, ok := findPatchType["properties"]; ok {
		var newValue externalpolicy.PropertyList
		patch := make(map[string]externalpolicy.PropertyList)
		err := json.Unmarshal([]byte(attribute), &patch)
		if err != nil {
			cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to unmarshal attribute input %s: %v", attribute, err)
		}
		newValue = patch["properties"]
		err = newValue.Validate()
		if err != nil {
			cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "Invalid format for properties")
		}
		patch["properties"] = newValue
		cliutils.ExchangePutPost("Exchange", http.MethodPatch, cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policyName), cliutils.OrgAndCreds(org, credToUse), []int{201}, patch)
		fmt.Println("Policy " + org + "/" + policyName + " updated in the Horizon Exchange")
	} else if _, ok := findPatchType["constraints"]; ok {
		var newValue externalpolicy.ConstraintExpression
		patch := make(map[string]externalpolicy.ConstraintExpression)
		err := json.Unmarshal([]byte(attribute), &patch)
		if err != nil {
			cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to unmarshal attribute input %s: %v", attribute, err)
		}
		err = newValue.Validate()
		if err != nil {
			cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "Invalid format for constraints: %v")
		}
		newValue = patch["constraints"]
		cliutils.ExchangePutPost("Exchange", http.MethodPatch, cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policyName), cliutils.OrgAndCreds(org, credToUse), []int{201}, patch)
		fmt.Println("Policy " + org + "/" + policyName + " updated in the Horizon Exchange")
	} else if _, ok := findPatchType["userInput"]; ok {
		patch := make(map[string][]policy.UserInput)
		err := json.Unmarshal([]byte(attribute), &patch)
		if err != nil {
			cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to unmarshal attribute input %s: %v", attribute, err)
		}
		cliutils.ExchangePutPost("Exchange", http.MethodPatch, cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policyName), cliutils.OrgAndCreds(org, credToUse), []int{201}, patch)
		fmt.Println("Policy " + org + "/" + policyName + " updated in the Horizon Exchange")
	} else {
		_, ok := findPatchType["label"]
		_, ok2 := findPatchType["description"]
		if ok || ok2 {
			patch := make(map[string]string)
			err := json.Unmarshal([]byte(attribute), &patch)
			if err != nil {
				cliutils.Fatal(cliutils.JSON_PARSING_ERROR, "failed to unmarshal attribute input %s: %v", attribute, err)
			}
			cliutils.ExchangePutPost("Exchange", http.MethodPatch, cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policyName), cliutils.OrgAndCreds(org, credToUse), []int{201}, patch)
			fmt.Println("Policy " + org + "/" + policyName + " updated in the Horizon Exchange")
		} else {
			cliutils.Fatal(cliutils.CLI_INPUT_ERROR, "Business policy attribute to be updated is not found in the input file. Supported attributes are: label, description, service, properties, constraints, and userInput.")
		}
	}
}

//BusinessRemovePolicy will remove an existing business policy in the Horizon Exchange
func BusinessRemovePolicy(org string, credToUse string, policy string, force bool) {
	cliutils.SetWhetherUsingApiKey(credToUse)
	org, credToUse = cliutils.TrimOrg(org, credToUse)
	if !force {
		cliutils.ConfirmRemove("Are you sure you want to remove business policy " + policy + " for org " + org + " from the Horizon Exchange?")
	}

	//check if policy name is passed in as <org>/<service>
	org, policy = cliutils.TrimOrg(org, policy)

	//remove policy
	httpCode := cliutils.ExchangeDelete("Exchange", cliutils.GetExchangeUrl(), "orgs/"+org+"/business/policies"+cliutils.AddSlash(policy), cliutils.OrgAndCreds(org, credToUse), []int{204, 404})
	if httpCode == 404 {
		fmt.Println("Policy " + org + "/" + policy + " not found in the Horizon Exchange")
	} else {
		fmt.Println("Business policy " + org + "/" + policy + " removed")
	}
}

// Display an empty business policy template as an object.
func BusinessNewPolicy() {
	fmt.Println(BUSINESS_POLICY_TEMPLATE_OBJECT)
}
