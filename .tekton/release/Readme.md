# Release Automation Tools

This directory hosts pipelines for release automation. There are 2 pipelines at the moment: bundle update and catalog update.

## Bundle Update

The pipeline to execute after component release is done.
It updates the related_images and make a pull request to lightspeed-operator repository.

### Parameters

| Name         | Description                                                                                      | Optional | Default Value                                         |
| ------------ | ------------------------------------------------------------------------------------------------ | -------- | ----------------------------------------------------- |
| snapshot     | The component snapshot to update the bundle with. Provided by Konflux.                           | No       | -                                                     |
| release      | The release name and namespace in the format of <namespace>/<release-name>. Provided by Konflux. | No       | -                                                     |
| releasePlan  | The release plan name, which is not used in this pipeline. Provided by Konflux.                  | Yes      | <https://github.com/konflux-ci/community-catalog.git> |
| forkGitUrl   | The git repository URL where the updated code is committed to.                                   | No       | -                                                     |
| forkBranch   | The branch of the git repository where the updated bundle is committed to.                       | No       | -                                                     |
| forkUser     | The Github user of the fork git repository.                                                      | No       | -                                                     |
| sourceGitUrl | The source git repository URL to clone as starting point for the bundle update.                  | No       | -                                                     |
| sourceBranch | The branch of the source git repository to clone as starting point for the bundle update.        | No       | -                                                     |

### Results
| Name             | Description                            |
| ---------------- | -------------------------------------- |
| commit-id        | The commit ID where bundle is updated. |
| pull-request-url | The URL of the created pull request.   |

## Catalog Update

The pipelien to execute after bundle release is done.
It updates the FBC for each supported Openshift version and make a pull request to lightspeed-operator repository.

### Parameters

| Name         | Description                                                                                      | Optional | Default Value                                         |
| ------------ | ------------------------------------------------------------------------------------------------ | -------- | ----------------------------------------------------- |
| snapshot     | The bundle snapshot to update the catalog with. Provided by Konflux.                             | No       | -                                                     |
| release      | The release name and namespace in the format of <namespace>/<release-name>. Provided by Konflux. | No       | -                                                     |
| releasePlan  | The release plan name, which is not used in this pipeline. Provided by Konflux.                  | Yes      | <https://github.com/konflux-ci/community-catalog.git> |
| forkGitUrl   | The git repository URL where the updated code is committed to.                                   | No       | -                                                     |
| forkBranch   | The branch of the git repository where the updated catalog is committed to.                      | No       | -                                                     |
| forkUser     | The Github user of the fork git repository.                                                      | No       | -                                                     |
| sourceGitUrl | The source git repository URL to clone as starting point for the bundle update.                  | No       | -                                                     |
| sourceBranch | The branch of the source git repository to clone as starting point for the bundle update.        | No       | -                                                     |

### Results
| Name             | Description                             |
| ---------------- | --------------------------------------- |
| commit-id        | The commit ID where catalog is updated. |
| pull-request-url | The URL of the created pull request.    |


## Service Account

These pipeline requires certain access to Konflux resources.check the service account role settings for detail. 

## Secrets

2 tokens should be provided through secret:

- Token of the service account to create objects in Konflux. 
- Tokne of a Github account to create pull requests to lightspeed-operator.

## Testing / Debugging

Use the pipelineruns to debug the respective pipeline.
