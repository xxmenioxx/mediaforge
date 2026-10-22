import { AssetDefaultsCard } from '../../components/AssetDefaultsCard';

export function AssetDefaultsSettingsDomain(props: Parameters<typeof AssetDefaultsCard>[0]) {
  return <AssetDefaultsCard {...props} key={JSON.stringify(props.value)} />;
}
