#!/usr/bin/env node
import { App } from 'aws-cdk-lib';
import { GarnetStack } from '../lib/garnet-stack';
import { Parameters } from '../parameters';

const app = new App();

new GarnetStack(app, 'Garnet', {
    stackName: 'Garnet',
    description: 'Garnet Framework is an open-source framework for building scalable, reliable and interoperable platforms - (uksb-1tupboc26)',
    env: { region: Parameters.aws_region }
})