#!/usr/bin/env python
#
# Generate an operator bundle for publishing to OLM. Copies appropriate files
# into a directory, and composes the ClusterServiceVersion which needs bits and
# pieces of our rbac and deployment files.
#

import datetime
import os
import sys
import yaml
import shutil
import subprocess

if __name__ == '__main__':

    if len(sys.argv) != 9:
        print("USAGE: %s OPERATOR_DIR OPERATOR_NAME OPERATOR_NAMESPACE OPERATOR_VERSION OPERATOR_IMAGE CHANNEL_NAME MULTI_NAMESPACE PREVIOUS_VERSION" % sys.argv[0])
        sys.exit(1)

    operator_dir = sys.argv[1]
    operator_name = sys.argv[2]
    operator_namespace = sys.argv[3]
    operator_version = sys.argv[4]
    operator_image = sys.argv[5]
    channel_name = sys.argv[6]
    # Coerce to a boolean
    multi_namespace = sys.argv[7] == "true".lower()
    prev_csv = sys.argv[8]

    operator_assets_dir = os.path.join("deploy")
    # Check to see if the deploy directory exists before going on.
    if not os.path.exists(operator_assets_dir):
        print("ERROR Operator asset directory {} does not exist. Giving up.".format(operator_assets_dir))
        sys.exit(1)

    version_dir = os.path.join(operator_dir, operator_version)
    if os.path.exists(version_dir):
        print("INFO version already exists, skipping: {}".format(version_dir))
        sys.exit(0)

    # doesn't exist, create the target version
    os.mkdir(version_dir)


    # create package content
    package = {}
    package['packageName'] = operator_name
    package['channels'] = []
    package['channels'].append({'currentCSV': "%s.v%s" % (operator_name, operator_version), 'name': channel_name})

    print("Generating CSV for version: %s" % operator_version)

    with open('hack/templates/csv.yaml', 'r') as stream:
        csv = yaml.safe_load(stream)

    # set templated values
    csv['metadata']['name'] = operator_name
    csv['metadata']['namespace'] = operator_namespace
    csv['metadata']['containerImage'] = operator_image
    csv['spec']['displayName'] = operator_name
    csv['spec']['description'] = "SRE operator - " + operator_name
    csv['spec']['version'] = operator_version

    # Initialize CRD array in CSV
    csv['spec']['customresourcedefinitions']['owned'] = []

    # Copy all CSV files over to the bundle output dir:
    # Copy all CRD files over to the bundle output dir:
    crd_files = [ f for f in os.listdir('deploy/crds') if f.endswith('_crd.yaml') ]
    for file_name in crd_files:
        full_path = os.path.join('deploy/crds', file_name)
        if (os.path.isfile(os.path.join('deploy/crds', file_name))):
            shutil.copy(full_path, os.path.join(version_dir, file_name))
        # Load CRD so we can use attributes from it
        with open("deploy/crds/{}".format(file_name), "r") as stream:
            crd = yaml.load(stream)
        # Update CSV template customresourcedefinitions key
        csv['spec']['customresourcedefinitions']['owned'].append(
            {
                "name": crd["metadata"]["name"],
                "description": crd["spec"]["names"]["kind"],
                "displayName": crd["spec"]["names"]["kind"],
                "kind": crd["spec"]["names"]["kind"],
                "version": crd["spec"]["version"]
            }
        )

    csv['spec']['install']['spec']['clusterPermissions'] = []

    SA_NAME = operator_name
    clusterrole_names_csv = []

    for subdir, dirs, files in os.walk(operator_assets_dir):
        for file in files:
            file_path = subdir + os.sep + file

            # Parse each file and look for ClusterRoleBindings to the SA
            with open(file_path) as stream:
                yaml_file = yaml.safe_load_all(stream)
                for obj in yaml_file:
                    if obj['kind'] == 'ClusterRoleBinding':
                        for subject in obj['subjects']:
                            if subject['kind'] == 'ServiceAccount' and subject['name'] == SA_NAME:
                                clusterrole_names_csv.append(obj['roleRef']['name'])

    csv['spec']['install']['spec']['deployments'] = []
    csv['spec']['install']['spec']['deployments'].append({'spec':{}})

    for subdir, dirs, files in os.walk(operator_assets_dir):
        for file in files:
            file_path = subdir + os.sep + file
            # Parse files to manage clusterPermissions and deployments in csv
            with open(file_path) as stream:
                yaml_file = yaml.safe_load_all(stream)
                for obj in yaml_file:
                    if obj['kind'] == 'ClusterRole' and any(obj['metadata']['name'] in cr for cr in clusterrole_names_csv):
                        print('Adding ClusterRole to CSV: {}'.format(file_path))
                        csv['spec']['install']['spec']['clusterPermissions'].append(
                        {
                            'rules': obj['rules'],
                            'serviceAccountName': SA_NAME,
                        })
                    if obj['kind'] == 'Deployment' and obj['metadata']['name'] == operator_name:
                        print('Adding Deployment to CSV: {}'.format(file_path))
                        csv['spec']['install']['spec']['deployments'][0]['spec'] = obj['spec']
                        csv['spec']['install']['spec']['deployments'][0]['name'] = operator_name
                    if obj['kind'] == 'ClusterRole' or obj['kind'] == 'Role' or obj['kind'] == 'RoleBinding' or obj['kind'] == 'ClusterRoleBinding':
                        if obj['kind'] in ('RoleBinding', 'ClusterRoleBinding'):
                            try:
                                print(obj['roleRef']['kind'])
                            except KeyError:
                                # require a well formed roleRef, olm doesn't check this until deployed and InstallPlan fails
                                print("ERROR {} '{}' is missing .roleRef.kind in file {}".format(obj['kind'], obj['metadata']['name'], file_path))
                                sys.exit(1)

                        print('Adding {} to Catalog: {}'.format(obj['kind'], file_path))
                        if 'namespace' in obj['metadata']:
                            bundle_filename="10-{}.{}.{}.yaml".format(obj['metadata']['namespace'], obj['metadata']['name'], obj['kind']).lower()
                        else:
                            bundle_filename="00-{}.{}.yaml".format(obj['metadata']['name'], obj['kind']).lower()
                        shutil.copyfile(file_path, os.path.join(version_dir, bundle_filename))

    if len(csv['spec']['install']['spec']['deployments']) == 0:
        print("ERROR Did not find any Deployments in {}. There is nothing to deploy, so giving up.".format(operator_assets_dir))
        sys.exit(1)


    # Update the deployment to use the defined image:
    csv['spec']['install']['spec']['deployments'][0]['spec']['template']['spec']['containers'][0]['image'] = operator_image

    # Update the versions to include git hash:
    csv['metadata']['name'] = "%s.v%s" % (operator_name, operator_version)
    csv['spec']['version'] = operator_version
    if prev_csv != "__undefined__":
        csv['spec']['replaces'] = prev_csv

    # adjust the install mode for multiple namespaces, if we need to
    i = 0
    found_multi_namespace = False
    for m in csv['spec']['installModes']:
        print("Looking for MultiNamespace, i = {} on = {}".format(i, m['type']))
        if m['type'] == "MultiNamespace":
            found_multi_namespace = True
            break
        i = i + 1
    
    if found_multi_namespace:
        csv['spec']['installModes'][i]['supported'] = multi_namespace

    # Set the CSV createdAt annotation:
    now = datetime.datetime.now()
    csv['metadata']['annotations']['createdAt'] = now.strftime("%Y-%m-%dT%H:%M:%SZ")

    # Write the CSV to disk:
    csv_filename = "20-%s.v%s.clusterserviceversion.yaml" % (operator_name, operator_version)
    csv_file = os.path.join(version_dir, csv_filename)
    with open(csv_file, 'w') as outfile:
        yaml.dump(csv, outfile, default_flow_style=False)
    print("Wrote ClusterServiceVersion: %s" % csv_file)
