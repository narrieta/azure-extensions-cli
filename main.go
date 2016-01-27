package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"text/template"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/olekukonko/tablewriter"
)

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stderr)
}

var (
	flManifest = cli.StringFlag{
		Name:  "manifest",
		Usage: "Path of XML manifest for extension package (output of new-extension-manifest)"}
	flSubsID = cli.StringFlag{
		Name:  "subscription-id",
		Usage: "Subscription ID for the publisher subscription"}
	flSubsCert = cli.StringFlag{
		Name:  "subscription-cert",
		Usage: "Path of subscription management certificate (.pem) file"}
	flVersion = cli.StringFlag{
		Name:  "version",
		Usage: "Version of the extension package e.g. 1.0.0"}
	flNamespace = cli.StringFlag{
		Name:  "namespace",
		Usage: "Publisher namespace e.g. Microsoft.Azure.Extensions"}
	flName = cli.StringFlag{
		Name:  "name",
		Usage: "Name of the extension e.g. FooExtension"}
)

func main() {
	app := cli.NewApp()
	app.Name = "azure-extensions-cli"
	app.Usage = "This tool is designed for Microsoft internal extension publishers to release, update and manage Virtual Machine extensions."
	app.Authors = []cli.Author{{Name: "Ahmet Alp Balkan", Email: "ahmetb at microsoft döt com"}}
	app.Commands = []cli.Command{
		{Name: "new-extension-manifest",
			Usage:  "Creates an XML file used to publish or update extension.",
			Action: newExtensionManifest,
			Flags: []cli.Flag{
				flNamespace,
				flName,
				flVersion,
				cli.StringFlag{
					Name:  "label",
					Usage: "Human readable name of the extension"},
				cli.StringFlag{
					Name:  "description",
					Usage: "Description of the extension"},
				cli.StringFlag{
					Name:  "eula-url",
					Usage: "URL to the End-User License Agreement page"},
				cli.StringFlag{
					Name:  "privacy-url",
					Usage: "URL to the Privacy Policy page"},
				cli.StringFlag{
					Name:  "homepage-url",
					Usage: "URL to the homepage of the extension"},
				cli.StringFlag{
					Name:  "company",
					Usage: "Human-readable Company Name of the publisher"},
				cli.StringFlag{
					Name:  "supported-os",
					Usage: "Extension platform e.g. 'Linux'"},
			},
		},
		{Name: "list-versions",
			Usage:  "Lists all published extension versions for subscription",
			Flags:  []cli.Flag{flSubsID, flSubsCert},
			Action: listVersions,
		},
		{Name: "replication-status",
			Usage:  "Retrieves replication status for an uploaded extension package",
			Flags:  []cli.Flag{flSubsID, flSubsCert, flNamespace, flName, flVersion},
			Action: replicationStatus,
		},
		{Name: "unpublish-version",
			Usage:  "Marks the specified version of the extension internal. Does not delete.",
			Flags:  []cli.Flag{flSubsID, flSubsCert, flNamespace, flName, flVersion},
			Action: unpublishVersion,
		},
		{Name: "delete-version",
			Usage:  "Deletes the extension version. It should be unpublished first.",
			Flags:  []cli.Flag{flSubsID, flSubsCert, flNamespace, flName, flVersion},
			Action: deleteVersion,
		},
	}
	app.RunAndExitOnError()
}

func newExtensionManifest(c *cli.Context) {
	var p struct {
		Namespace, Name, Version, Label, Description, Eula, Privacy, Homepage, Company, OS string
	}
	flags := []struct {
		ref *string
		fl  string
	}{
		{&p.Namespace, flNamespace.Name},
		{&p.Name, flName.Name},
		{&p.Version, flVersion.Name},
		{&p.Label, "label"},
		{&p.Description, "description"},
		{&p.Eula, "eula-url"},
		{&p.Privacy, "privacy-url"},
		{&p.Homepage, "homepage-url"},
		{&p.Company, "company"},
		{&p.OS, "supported-os"},
	}
	for _, f := range flags {
		*f.ref = checkFlag(c, f.fl)
	}
	// doing a text template is easier and let us create comments (xml encoder can't)
	// that are used as placeholders later on.
	manifestXml := `<?xml version="1.0" encoding="utf-8" ?>
<ExtensionImage xmlns="http://schemas.microsoft.com/windowsazure"  xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
  <!-- WARNING: Ordering of fields matter in this file. -->
  <ProviderNameSpace>{{.Namespace}}</ProviderNameSpace>
  <Type>{{.Name}}</Type>
  <Version>{{.Version}}</Version>
  <Label>{{.Label}}</Label>
  <HostingResources>VmRole</HostingResources>
  <MediaLink>%BLOB_URL%</MediaLink>
  <Description>{{.Description}}</Description>
  <IsInternalExtension>true</IsInternalExtension>
  <Eula>{{.Eula}}</Eula>
  <PrivacyUri>{{.Privacy}}</PrivacyUri>
  <HomepageUri>{{.Homepage}}</HomepageUri>
  <IsJsonExtension>true</IsJsonExtension>
  <CompanyName>{{.Company}}</CompanyName>
  <SupportedOS>{{.OS}}</SupportedOS>
  <!--%REGIONS%-->
</ExtensionImage>
`
	tpl, err := template.New("manifest").Parse(manifestXml)
	if err != nil {
		log.Fatalf("template parse error: %v", err)
	}
	if err = tpl.Execute(os.Stdout, p); err != nil {
		log.Fatalf("template execute error: %v", err)
	}
}

func listVersions(c *cli.Context) {
	cl := mkClient(checkFlag(c, flSubsID.Name), checkFlag(c, flSubsCert.Name))
	v, err := cl.ListVersions()
	if err != nil {
		log.Fatal("Request failed: %v", err)
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Namespace", "Type", "Version", "Replication Completed", "Regions"})
	data := [][]string{}
	for _, e := range v.Extensions {
		data = append(data, []string{e.Ns, e.Name, e.Version, fmt.Sprintf("%v", e.ReplicationCompleted), e.Regions})
	}
	table.AppendBulk(data)
	table.Render()
}

func replicationStatus(c *cli.Context) {
	cl := mkClient(checkFlag(c, flSubsID.Name), checkFlag(c, flSubsCert.Name))
	ns, name, version := checkFlag(c, flNamespace.Name), checkFlag(c, flName.Name), checkFlag(c, flVersion.Name)
	log.Debug("Requesting replication status.")
	rs, err := cl.GetReplicationStatus(ns, name, version)
	if err != nil {
		log.Fatal("Cannot fetch replication status: %v", err)
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Location", "Status"})
	data := [][]string{}
	for _, s := range rs.Statuses {
		data = append(data, []string{s.Location, s.Status})
	}
	table.AppendBulk(data)
	table.Render()
}

func unpublishVersion(c *cli.Context) {
	p := struct {
		Namespace, Name, Version string
	}{
		Namespace: checkFlag(c, flNamespace.Name),
		Name:      checkFlag(c, flName.Name),
		Version:   checkFlag(c, flVersion.Name)}

	manifestXml := `<?xml version="1.0" encoding="utf-8" ?>
<ExtensionImage xmlns="http://schemas.microsoft.com/windowsazure"  xmlns:i="http://www.w3.org/2001/XMLSchema-instance">
  <!-- WARNING: Ordering of fields matter in this file. -->
  <ProviderNameSpace>{{.Namespace}}</ProviderNameSpace>
  <Type>{{.Name}}</Type>
  <Version>{{.Version}}</Version>
  <IsInternalExtension>true</IsInternalExtension>
  <IsJsonExtension>true</IsJsonExtension>
</ExtensionImage>`
	tpl, err := template.New("unregisterManifest").Parse(manifestXml)
	if err != nil {
		log.Fatalf("template parse error: %v", err)
	}

	var b bytes.Buffer
	if err = tpl.Execute(&b, p); err != nil {
		log.Fatalf("template execute error: %v", err)
	}

	cl := mkClient(checkFlag(c, flSubsID.Name), checkFlag(c, flSubsCert.Name))
	op, err := cl.UpdateExtension(b.Bytes())
	if err != nil {
		log.Fatalf("UpdateExtension failed: %v", err)
	}
	lg := log.WithField("x-ms-operation-id", op)
	lg.Info("UpdateExtension operation started.")
	if err := cl.WaitForOperation(op); err != nil {
		lg.Fatalf("UpdateExtension failed: %v", err)
	}
	lg.Info("UpdateExtension operation finished.")
}

func deleteVersion(c *cli.Context) {
	cl := mkClient(checkFlag(c, flSubsID.Name), checkFlag(c, flSubsCert.Name))
	ns, name, version := checkFlag(c, flNamespace.Name), checkFlag(c, flName.Name), checkFlag(c, flVersion.Name)
	log.Info("Deleting extension version. Make sure you unpublished before deleting.")

	op, err := cl.DeleteExtension(ns, name, version)
	if err != nil {
		log.Fatalf("Error deleting version: %v", err)
	}
	log.Debug("DeleteExtension operation started.")
	if err := cl.WaitForOperation(op); err != nil {
		log.Fatalf("DeleteExtension failed: %v", err)
	}
	log.Info("DeleteExtension operation finished.")
}

func mkClient(subscriptionID, certFile string) ExtensionsClient {
	b, err := ioutil.ReadFile(certFile)
	if err != nil {
		log.Fatal("Cannot read certificate %s: %v", certFile, err)
	}
	cl, err := NewClient(subscriptionID, b)
	if err != nil {
		log.Fatal("Cannot create client: %v", err)
	}
	return cl
}

func checkFlag(c *cli.Context, fl string) string {
	v := c.String(fl)
	if v == "" {
		log.Fatalf("argument %s must be provided", fl)
	}
	return v
}
