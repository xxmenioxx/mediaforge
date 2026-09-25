import type { LabFidelityOperation } from '../api/types';

type LabFidelityRunIdentity = {
  operationId: string;
  primaryRequestId: string;
  primaryArtifactId: string;
  primaryGeneration: string;
  materialSignature: string;
  localRequestSignature: string;
};

export function labFidelityOperationMatchesRun(operation: LabFidelityOperation, run: LabFidelityRunIdentity) {
  return operation.id === run.operationId &&
    operation.primaryRequestId === run.primaryRequestId &&
    operation.primaryArtifactId === run.primaryArtifactId &&
    operation.primaryGeneration === run.primaryGeneration &&
    operation.materialSignature === run.materialSignature;
}
